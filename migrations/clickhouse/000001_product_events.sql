CREATE TABLE IF NOT EXISTS product_events (
    event_id String,
    event_type LowCardinality(String),
    occurred_at DateTime64(3, 'UTC'),
    producer LowCardinality(String),
    trace_id String,
    journey_id String,
    recommendation_id String,
    user_id_hash String,
    offer_id String,
    redemption_id String,
    merchant_id String,
    copy_source LowCardinality(String),
    experiment_id String,
    variant String,
    attribution_type LowCardinality(String),
    payload String,
    ingested_at DateTime64(3, 'UTC') DEFAULT now64(3)
) ENGINE = ReplacingMergeTree(ingested_at)
ORDER BY event_id;

CREATE VIEW IF NOT EXISTS journey_funnel AS
SELECT
    journey_id,
    minIf(occurred_at, event_type = 'journey.entered.v1') AS entered_at,
    minIf(occurred_at, event_type = 'recommendation.created.v1') AS recommended_at,
    minIf(occurred_at, event_type = 'recommendation.impressed.v1') AS impressed_at,
    minIf(occurred_at, event_type = 'recommendation.clicked.v1') AS clicked_at,
    minIf(occurred_at, event_type = 'redemption.succeeded.v1') AS redeemed_at,
    minIf(occurred_at, event_type = 'merchant.verified.v1') AS verified_at,
    anyHeavy(copy_source) AS copy_source,
    anyHeavy(experiment_id) AS experiment_id,
    anyHeavy(variant) AS variant,
    anyHeavy(attribution_type) AS attribution_type,
    minIf(occurred_at, event_type = 'visit.attributed.v1') AS attributed_visit_at
FROM product_events FINAL
WHERE journey_id != ''
GROUP BY journey_id;
