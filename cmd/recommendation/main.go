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
	"github.com/redis/go-redis/v9"

	"spacetime-node/internal/platform/app"
	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/observability"
	"spacetime-node/internal/platform/outbox"
	"spacetime-node/internal/recommendation"
)

var version = "dev"

func main() {
	dependencies := config.LoadDependencies()
	if dependencies.PostgresDSN == "" || dependencies.KafkaBrokers == "" || dependencies.QdrantURL == "" {
		log.Fatal("POSTGRES_DSN, KAFKA_BROKERS, and QDRANT_URL are required")
	}
	db, err := sql.Open("pgx", dependencies.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var redisClient *redis.Client
	if dependencies.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: dependencies.RedisAddr})
		defer redisClient.Close()
	}
	qdrant := recommendation.NewQdrantClient(dependencies.QdrantURL, nil)
	service := recommendation.NewRecommendationService(
		db,
		qdrant,
		recommendation.NewPreferenceStore(redisClient, recommendation.PreferenceTTL).WithDB(db),
	)
	if dependencies.LLMMode == "provider" && dependencies.LLMBaseURL != "" && dependencies.LLMModel != "" {
		service.WithCopyGenerator(recommendation.NewHTTPCopyGenerator(dependencies.LLMBaseURL, dependencies.LLMModel, nil), recommendation.DefaultCopyTimeout)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := observability.NewLogger("recommendation-service")
	publisher := outbox.NewKafkaPublisher(db, strings.Split(dependencies.KafkaBrokers, ","))
	go outbox.RunPublisher(ctx, publisher, 100, 100*time.Millisecond, logger)
	go func() {
		if err := recommendation.RunEntryConsumer(ctx, strings.Split(dependencies.KafkaBrokers, ","), service, logger); err != nil && ctx.Err() == nil {
			logger.Printf("entry consumer stopped: %v", err)
		}
	}()

	if err := app.Run("recommendation-service", ":8002", ":9002", version, db.PingContext, qdrant.Ping); err != nil {
		log.Fatal(err)
	}
}
