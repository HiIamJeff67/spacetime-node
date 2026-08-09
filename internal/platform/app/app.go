package app

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"spacetime-node/internal/platform/config"

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

func Run(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr, version string) error {
	return RunWithSetup(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr, version, nil)
}

func RunWithSetup(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr, version string, setup Setup) error {
	runtime := config.Load(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr)
	httpServer := khttp.NewServer(
		khttp.Address(runtime.HTTPAddr),
		khttp.Timeout(5*time.Second),
		khttp.Middleware(recovery.Recovery()),
	)
	httpServer.Handle("/healthz", statusHandler(runtime.ServiceName, version, "ok"))
	httpServer.Handle("/version", statusHandler(runtime.ServiceName, version, "ready"))
	grpcServer := kgrpc.NewServer(
		kgrpc.Address(runtime.GRPCAddr),
		kgrpc.Middleware(recovery.Recovery()),
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
		kratos.Server(httpServer, grpcServer),
	).Run()
}

func statusHandler(service, version, state string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status{
			Service: service,
			Version: version,
			Status:  state,
		})
	})
}
