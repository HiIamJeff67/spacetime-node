package main

import (
	"log"
	"time"

	"github.com/redis/go-redis/v9"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
	"spacetime-node/internal/mobility"
	"spacetime-node/internal/platform/app"
	"spacetime-node/internal/platform/config"

	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

var version = "dev"

func main() {
	dependencies := config.LoadDependencies()
	provider := mobility.NewHTTPProvider(dependencies.BeaconURL, dependencies.BeaconUser, dependencies.BeaconPassword, time.Duration(dependencies.BeaconTimeoutMS)*time.Millisecond)
	var redisClient *redis.Client
	if dependencies.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: dependencies.RedisAddr})
		defer redisClient.Close()
	}
	resolver := mobility.DefaultResolver(provider, redisClient)
	if err := app.RunWithSetup("mobility-service", ":8001", ":9001", version, func(httpServer *khttp.Server, grpcServer *kgrpc.Server) error {
		service := mobility.NewService(resolver)
		v1.RegisterMobilityServiceHTTPServer(httpServer, service)
		v1.RegisterMobilityServiceServer(grpcServer, service)
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
