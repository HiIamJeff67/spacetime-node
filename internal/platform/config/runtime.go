package config

import "os"

type Runtime struct {
	ServiceName string
	HTTPAddr    string
	GRPCAddr    string
}

// Dependencies contains the shared runtime connection settings that every
// service receives from its environment. Services use only the fields they own;
// this package is the single place that maps environment variables. Do not log
// this value because a DSN may include credentials.
type Dependencies struct {
	KafkaBrokers  string
	PostgresDSN   string
	RedisAddr     string
	ClickHouseDSN string
	QdrantURL     string
	LLMMode       string
	LLMBaseURL    string
	LLMModel      string
}

func Load(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr string) Runtime {
	return Runtime{
		ServiceName: envOrDefault("SERVICE_NAME", defaultServiceName),
		HTTPAddr:    envOrDefault("HTTP_ADDR", defaultHTTPAddr),
		GRPCAddr:    envOrDefault("GRPC_ADDR", defaultGRPCAddr),
	}
}

func LoadDependencies() Dependencies {
	return Dependencies{
		KafkaBrokers:  os.Getenv("KAFKA_BROKERS"),
		PostgresDSN:   os.Getenv("POSTGRES_DSN"),
		RedisAddr:     os.Getenv("REDIS_ADDR"),
		ClickHouseDSN: os.Getenv("CLICKHOUSE_DSN"),
		QdrantURL:     os.Getenv("QDRANT_URL"),
		LLMMode:       envOrDefault("LLM_MODE", "template"),
		LLMBaseURL:    os.Getenv("LLM_BASE_URL"),
		LLMModel:      os.Getenv("LLM_MODEL"),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
