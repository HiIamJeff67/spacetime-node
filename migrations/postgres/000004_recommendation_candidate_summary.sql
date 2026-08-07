ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS candidate_summary JSONB NOT NULL DEFAULT '[]'::jsonb
    CHECK (jsonb_typeof(candidate_summary) = 'array');
