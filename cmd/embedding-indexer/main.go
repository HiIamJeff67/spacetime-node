package main

import (
	"context"
	"database/sql"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/segmentio/kafka-go"

	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/observability"
	"spacetime-node/internal/recommendation"
)

var version = "dev"

func main() {
	dependencies := config.LoadDependencies()
	if dependencies.KafkaBrokers == "" || dependencies.PostgresDSN == "" || dependencies.QdrantURL == "" {
		log.Fatal("KAFKA_BROKERS, POSTGRES_DSN, and QDRANT_URL are required")
	}
	db, err := sql.Open("pgx", dependencies.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTelemetry, err := observability.Setup(ctx, "embedding-indexer", version)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTelemetry(shutdownCtx)
	}()
	embedder, err := recommendation.NewConfiguredEmbedder(
		dependencies.EmbeddingMode,
		dependencies.EmbeddingBaseURL,
		dependencies.EmbeddingModel,
		dependencies.EmbeddingDimension,
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}
	indexer := recommendation.NewOfferIndexer(
		db,
		recommendation.NewQdrantClient(dependencies.QdrantURL, nil),
		embedder,
		dependencies.EmbeddingModel,
	).WithCollection(dependencies.EmbeddingCollection)
	if err := indexer.Bootstrap(ctx, dependencies.EmbeddingDimension); err != nil {
		log.Fatal(err)
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  strings.Split(dependencies.KafkaBrokers, ","),
		Topic:    "offer.changed.v1",
		GroupID:  "embedding-indexer-v1",
		MinBytes: 1,
		MaxBytes: 10e6,
	})
	defer reader.Close()
	logger := observability.NewLogger("embedding-indexer")
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Fatal(err)
		}
		eventCtx := observability.ExtractKafka(ctx, message.Headers)
		eventCtx, span := observability.StartKafkaSpan(eventCtx, "offer.changed.v1", "", true)
		if err := indexer.HandleOfferChanged(eventCtx, message.Value); err != nil {
			span.RecordError(err)
			span.End()
			logger.Fatal(err)
		}
		if err := reader.CommitMessages(eventCtx, message); err != nil {
			span.RecordError(err)
			span.End()
			logger.Fatal(err)
		}
		span.End()
	}
}
