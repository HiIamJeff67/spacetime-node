package main

import (
	"context"
	"database/sql"
	"log"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/segmentio/kafka-go"

	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/recommendation"
)

const embeddingDimension = 32

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
	indexer := recommendation.NewOfferIndexer(
		db,
		recommendation.NewQdrantClient(dependencies.QdrantURL, nil),
		recommendation.HashEmbedder(embeddingDimension),
		"demo-hash-v1",
	)
	if err := indexer.Bootstrap(ctx, embeddingDimension); err != nil {
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
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Fatal(err)
		}
		if err := indexer.HandleOfferChanged(ctx, message.Value); err != nil {
			log.Fatal(err)
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			log.Fatal(err)
		}
	}
}
