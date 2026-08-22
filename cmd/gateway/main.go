package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v3/transport/grpc"
	"github.com/go-kratos/kratos/v3/transport/http"
	_ "github.com/jackc/pgx/v5/stdlib"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
	"spacetime-node/internal/journey"
	"spacetime-node/internal/mobility"
	"spacetime-node/internal/notification"
	"spacetime-node/internal/platform/app"
	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/observability"
	"spacetime-node/internal/platform/outbox"
	"spacetime-node/internal/redemption"
	"spacetime-node/internal/user"
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

	var beaconResolver mobility.Client
	if strings.TrimSpace(dependencies.MobilityURL) != "" {
		client, err := mobility.NewHTTPClient(dependencies.MobilityURL, time.Duration(dependencies.MobilityTimeoutMS)*time.Millisecond)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()
		beaconResolver = client
	}
	service := journey.NewService(db, beaconResolver).WithRecommendationCandidateLimit(dependencies.RecommendationCandidateLimit)
	userService := user.NewService(db)
	redemptionAPI := redemption.NewAPI(redemption.NewService(db))
	notificationAPI := notification.NewAPI(notification.NewService(db))
	publisher := outbox.NewKafkaPublisher(db, strings.Split(dependencies.KafkaBrokers, ","))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go outbox.RunPublisher(ctx, publisher, 100, 100*time.Millisecond, observability.NewLogger("gateway-service"))

	setup := func(httpServer *http.Server, grpcServer *grpc.Server) error {
		v1.RegisterJourneyServiceHTTPServer(httpServer, service)
		v1.RegisterJourneyServiceServer(grpcServer, service)
		v1.RegisterUserServiceHTTPServer(httpServer, userService)
		v1.RegisterUserServiceServer(grpcServer, userService)
		// Keep browser-facing redemption routes on the same public gateway origin
		// as entry, user, and recommendation APIs. The dedicated redemption
		// service remains available internally/on :8003 for service separation.
		v1.RegisterRedemptionServiceHTTPServer(httpServer, redemptionAPI)
		v1.RegisterRedemptionServiceServer(grpcServer, redemptionAPI)
		v1.RegisterNotificationServiceHTTPServer(httpServer, notificationAPI)
		v1.RegisterNotificationServiceServer(grpcServer, notificationAPI)
		return nil
	}
	if err := app.RunWithSetup("gateway-service", ":8000", ":9000", version, setup, db.PingContext); err != nil {
		log.Fatal(err)
	}
}
