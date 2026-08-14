package main

import (
	"context"
	"database/sql"
	"log"
	"os/signal"
	"strings"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"

	"spacetime-node/internal/notification"
	"spacetime-node/internal/platform/app"
	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/observability"
)

var version = "dev"

func main() {
	dependencies := config.LoadDependencies()
	if dependencies.PostgresDSN == "" || dependencies.KafkaBrokers == "" {
		log.Fatal("POSTGRES_DSN and KAFKA_BROKERS are required")
	}
	db, err := sql.Open("pgx", dependencies.PostgresDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := observability.NewLogger("notification-worker")
	go func() {
		if err := notification.Run(ctx, strings.Split(dependencies.KafkaBrokers, ","), db, logger); err != nil && ctx.Err() == nil {
			logger.Printf("notification worker stopped: %v", err)
		}
	}()
	if err := app.Run("notification-worker", ":8004", ":9004", version, db.PingContext); err != nil {
		log.Fatal(err)
	}
}
