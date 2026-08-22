package config

import (
	"os"
	"strconv"
)

type Runtime struct {
	ServiceName string
	HTTPAddr    string
	GRPCAddr    string
	// CORSAllowedOrigins is a comma-separated allowlist for browser origins.
	// It is intentionally empty by default; deployments must opt in explicitly.
	CORSAllowedOrigins string
}

// Dependencies contains the shared runtime connection settings that every
// service receives from its environment. Services use only the fields they own;
// this package is the single place that maps environment variables. Do not log
// this value because a DSN may include credentials.
type Dependencies struct {
	KafkaBrokers                 string
	PostgresDSN                  string
	RedisAddr                    string
	ClickHouseDSN                string
	QdrantURL                    string
	LLMMode                      string
	LLMBaseURL                   string
	LLMModel                     string
	EmbeddingMode                string
	EmbeddingBaseURL             string
	EmbeddingModel               string
	EmbeddingDimension           int
	EmbeddingCollection          string
	RecommendationCandidateLimit int
	NotificationProvider         string
	VAPIDSubject                 string
	VAPIDPublicKey               string
	VAPIDPrivateKey              string
	BeaconURL                    string
	BeaconUser                   string
	BeaconPassword               string
	BeaconTimeoutMS              int
	MobilityURL                  string
	MobilityTimeoutMS            int
}

func Load(defaultServiceName, defaultHTTPAddr, defaultGRPCAddr string) Runtime {
	runtime := Runtime{
		ServiceName: defaultServiceName,
		HTTPAddr:    defaultHTTPAddr,
		GRPCAddr:    defaultGRPCAddr,
	}
	if value := os.Getenv("SERVICE_NAME"); value != "" {
		runtime.ServiceName = value
	}
	if value := os.Getenv("HTTP_ADDR"); value != "" {
		runtime.HTTPAddr = value
	}
	if value := os.Getenv("GRPC_ADDR"); value != "" {
		runtime.GRPCAddr = value
	}
	runtime.CORSAllowedOrigins = os.Getenv("CORS_ALLOWED_ORIGINS")
	return runtime
}

func LoadDependencies() Dependencies {
	dependencies := Dependencies{
		KafkaBrokers:                 os.Getenv("KAFKA_BROKERS"),
		PostgresDSN:                  os.Getenv("POSTGRES_DSN"),
		RedisAddr:                    os.Getenv("REDIS_ADDR"),
		ClickHouseDSN:                os.Getenv("CLICKHOUSE_DSN"),
		QdrantURL:                    os.Getenv("QDRANT_URL"),
		LLMMode:                      "template",
		LLMBaseURL:                   os.Getenv("LLM_BASE_URL"),
		LLMModel:                     os.Getenv("LLM_MODEL"),
		EmbeddingMode:                "hash",
		EmbeddingBaseURL:             os.Getenv("EMBEDDING_BASE_URL"),
		EmbeddingModel:               "demo-hash-v1",
		EmbeddingDimension:           32,
		EmbeddingCollection:          "offer_embeddings_v1",
		RecommendationCandidateLimit: 10,
		NotificationProvider:         "mock",
		VAPIDSubject:                 os.Getenv("VAPID_SUBJECT"),
		VAPIDPublicKey:               os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey:              os.Getenv("VAPID_PRIVATE_KEY"),
		BeaconURL:                    os.Getenv("BEACON_API_URL"),
		BeaconUser:                   os.Getenv("BEACON_API_USERNAME"),
		BeaconPassword:               os.Getenv("BEACON_API_PASSWORD"),
		BeaconTimeoutMS:              800,
		MobilityURL:                  os.Getenv("MOBILITY_API_URL"),
		MobilityTimeoutMS:            800,
	}
	if value := os.Getenv("BEACON_API_TIMEOUT_MS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			dependencies.BeaconTimeoutMS = parsed
		}
	}
	if value := os.Getenv("MOBILITY_API_TIMEOUT_MS"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			dependencies.MobilityTimeoutMS = parsed
		}
	}
	if value := os.Getenv("LLM_MODE"); value != "" {
		dependencies.LLMMode = value
	}
	if value := os.Getenv("EMBEDDING_MODE"); value != "" {
		dependencies.EmbeddingMode = value
	}
	if value := os.Getenv("EMBEDDING_MODEL"); value != "" {
		dependencies.EmbeddingModel = value
	}
	if value := os.Getenv("EMBEDDING_DIMENSION"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			dependencies.EmbeddingDimension = parsed
		}
	}
	if value := os.Getenv("EMBEDDING_COLLECTION"); value != "" {
		dependencies.EmbeddingCollection = value
	}
	if value := os.Getenv("RECOMMENDATION_CANDIDATE_LIMIT"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
			dependencies.RecommendationCandidateLimit = parsed
		}
	}
	if value := os.Getenv("NOTIFICATION_PROVIDER"); value != "" {
		dependencies.NotificationProvider = value
	}
	return dependencies
}
