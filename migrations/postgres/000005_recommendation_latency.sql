ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS decision_latency_ms BIGINT NOT NULL DEFAULT 0
    CHECK (decision_latency_ms >= 0);
