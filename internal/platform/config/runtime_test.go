package config

import "testing"

func TestLoad(t *testing.T) {
	t.Setenv("SERVICE_NAME", "test-service")
	t.Setenv("HTTP_ADDR", ":18000")
	t.Setenv("GRPC_ADDR", ":19000")

	got := Load("fallback-service", ":8000", ":9000")
	if got.ServiceName != "test-service" || got.HTTPAddr != ":18000" || got.GRPCAddr != ":19000" {
		t.Fatalf("Load() = %+v", got)
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("SERVICE_NAME", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("GRPC_ADDR", "")

	got := Load("fallback-service", ":8000", ":9000")
	if got.ServiceName != "fallback-service" || got.HTTPAddr != ":8000" || got.GRPCAddr != ":9000" {
		t.Fatalf("Load() = %+v", got)
	}
}

func TestLoadDependencies(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("POSTGRES_DSN", "postgres://example")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("CLICKHOUSE_DSN", "clickhouse://clickhouse:9000/default")
	t.Setenv("QDRANT_URL", "http://qdrant:6333")
	t.Setenv("LLM_MODE", "provider")
	t.Setenv("LLM_BASE_URL", "http://llm:8080")
	t.Setenv("LLM_MODEL", "small-model")
	t.Setenv("EMBEDDING_MODE", "http")
	t.Setenv("EMBEDDING_BASE_URL", "http://embedding:8080")
	t.Setenv("EMBEDDING_MODEL", "multilingual-e5")
	t.Setenv("EMBEDDING_DIMENSION", "384")
	t.Setenv("EMBEDDING_COLLECTION", "offer_embeddings_v2")
	t.Setenv("RECOMMENDATION_CANDIDATE_LIMIT", "12")
	t.Setenv("NOTIFICATION_PROVIDER", "webpush")
	t.Setenv("VAPID_SUBJECT", "mailto:demo@example.com")
	t.Setenv("VAPID_PUBLIC_KEY", "public-key")
	t.Setenv("VAPID_PRIVATE_KEY", "private-key")

	got := LoadDependencies()
	if got.KafkaBrokers != "kafka:9092" || got.PostgresDSN != "postgres://example" || got.RedisAddr != "redis:6379" || got.ClickHouseDSN != "clickhouse://clickhouse:9000/default" || got.QdrantURL != "http://qdrant:6333" || got.LLMMode != "provider" || got.LLMBaseURL != "http://llm:8080" || got.LLMModel != "small-model" || got.EmbeddingMode != "http" || got.EmbeddingBaseURL != "http://embedding:8080" || got.EmbeddingModel != "multilingual-e5" || got.EmbeddingDimension != 384 || got.EmbeddingCollection != "offer_embeddings_v2" || got.RecommendationCandidateLimit != 12 || got.NotificationProvider != "webpush" || got.VAPIDSubject != "mailto:demo@example.com" || got.VAPIDPublicKey != "public-key" || got.VAPIDPrivateKey != "private-key" {
		t.Fatalf("LoadDependencies() = %+v", got)
	}
}
