package app

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"spacetime-node/internal/platform/config"
	"spacetime-node/internal/platform/observability"

	"github.com/go-kratos/kratos/v3"
	"github.com/go-kratos/kratos/v3/middleware/recovery"
	kgrpc "github.com/go-kratos/kratos/v3/transport/grpc"
	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

type status struct {
	Service string `json:"service"`
	Version string `json:"version"`
	Status  string `json:"status"`
}

type Setup func(*khttp.Server, *kgrpc.Server) error
type ReadyCheck func(context.Context) error

func Run(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr, version string, readyChecks ...ReadyCheck) error {
	return RunWithSetup(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr, version, nil, readyChecks...)
}

func RunWithSetup(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr, version string, setup Setup, readyChecks ...ReadyCheck) error {
	runtime := config.Load(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr)
	shutdownTelemetry, err := observability.Setup(context.Background(), runtime.ServiceName, version)
	if err != nil {
		return err
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTelemetry(ctx)
	}()
	telemetryMiddleware := observability.Middleware(runtime.ServiceName)
	httpServer := khttp.NewServer(
		khttp.Address(runtime.HTTPAddr),
		khttp.Timeout(5*time.Second),
		khttp.Filter(corsFilter(runtime.CORSAllowedOrigins)),
		khttp.Middleware(recovery.Recovery(), telemetryMiddleware),
	)
	httpServer.Handle("/healthz", statusHandler(runtime.ServiceName, version, "ok"))
	httpServer.Handle("/readyz", readinessHandler(runtime.ServiceName, version, readyChecks...))
	httpServer.Handle("/version", statusHandler(runtime.ServiceName, version, "ok"))
	grpcServer := kgrpc.NewServer(
		kgrpc.Address(runtime.GRPCAddr),
		kgrpc.Middleware(recovery.Recovery(), telemetryMiddleware),
	)
	if setup != nil {
		if err := setup(httpServer, grpcServer); err != nil {
			return err
		}
	}

	id, err := os.Hostname()
	if err != nil {
		id = runtime.ServiceName
	}

	return kratos.New(
		kratos.ID(id),
		kratos.Name(runtime.ServiceName),
		kratos.Version(version),
		kratos.StopTimeout(10*time.Second),
		kratos.Server(httpServer, grpcServer),
	).Run()
}

func readinessHandler(service, version string, checks ...ReadyCheck) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		for _, check := range checks {
			if check != nil && check(ctx) != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				writeStatus(w, service, version, "not_ready")
				return
			}
		}
		writeStatus(w, service, version, "ready")
	})
}

func statusHandler(service, version, state string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, service, version, state)
	})
}

func writeStatus(w http.ResponseWriter, service, version, state string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status{Service: service, Version: version, Status: state})
}
