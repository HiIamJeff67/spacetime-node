package main

import (
	"context"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"spacetime-node/internal/analytics"
	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/observability"
)

var version = "dev"

func main() {
	dependencies := config.LoadDependencies()
	if dependencies.KafkaBrokers == "" || dependencies.ClickHouseDSN == "" {
		log.Fatal("KAFKA_BROKERS and CLICKHOUSE_DSN are required")
	}
	clickhouse, err := analytics.NewClickHouse(dependencies.ClickHouseDSN, nil)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownTelemetry, err := observability.Setup(ctx, "analytics-consumer", version)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTelemetry(shutdownCtx)
	}()
	if err := analytics.Run(ctx, strings.Split(dependencies.KafkaBrokers, ","), clickhouse, observability.NewLogger("analytics-consumer")); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
