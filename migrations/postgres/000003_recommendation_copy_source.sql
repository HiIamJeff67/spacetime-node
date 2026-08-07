ALTER TABLE recommendations
    ADD COLUMN IF NOT EXISTS copy_source TEXT NOT NULL DEFAULT 'template'
    CHECK (copy_source IN ('template', 'llm'));
