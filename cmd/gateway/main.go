package main

import (
	"context"
	"database/sql"
	"log"
	"strings"

	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"
	_ "github.com/jackc/pgx/v5/stdlib"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
	"spacetime-node/internal/journey"
	"spacetime-node/internal/platform/app"
	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/outbox"
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

	service := journey.NewService(db)
	publisher := outbox.NewKafkaPublisher(db, strings.Split(dependencies.KafkaBrokers, ","))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go outbox.RunPublisher(ctx, publisher, 100, 0, log.Default())

	setup := func(httpServer *http.Server, grpcServer *grpc.Server) error {
		v1.RegisterJourneyServiceHTTPServer(httpServer, service)
		v1.RegisterJourneyServiceServer(grpcServer, service)
		return nil
	}
	if err := app.RunWithSetup("gateway-service", ":8000", ":9000", version, setup); err != nil {
		log.Fatal(err)
	}
}
