ALTER TABLE product_events
    ADD COLUMN IF NOT EXISTS attribution_type LowCardinality(String) DEFAULT 'none' AFTER variant;
