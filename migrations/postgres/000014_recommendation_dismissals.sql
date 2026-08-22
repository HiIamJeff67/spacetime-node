CREATE TABLE IF NOT EXISTS recommendation_dismissals (
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    journey_id TEXT NOT NULL REFERENCES journeys(journey_id) ON DELETE CASCADE,
    recommendation_id TEXT NOT NULL REFERENCES recommendations(recommendation_id) ON DELETE CASCADE,
    offer_id TEXT NOT NULL REFERENCES offers(offer_id),
    dismissed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, journey_id, recommendation_id, offer_id)
);

CREATE INDEX IF NOT EXISTS recommendation_dismissals_lookup_idx
ON recommendation_dismissals (journey_id, recommendation_id, offer_id);

INSERT INTO recommendation_dismissals (user_id, journey_id, recommendation_id, offer_id, dismissed_at)
SELECT
    j.user_id,
    o.payload->>'journey_id',
    o.payload->>'recommendation_id',
    o.payload->'payload'->>'offer_id',
    min(o.occurred_at)
FROM outbox_events o
JOIN journeys j ON j.journey_id = o.payload->>'journey_id'
WHERE o.topic = 'recommendation.dismissed.v1'
  AND o.payload->>'recommendation_id' IS NOT NULL
  AND o.payload->'payload'->>'offer_id' IS NOT NULL
GROUP BY
    j.user_id,
    o.payload->>'journey_id',
    o.payload->>'recommendation_id',
    o.payload->'payload'->>'offer_id'
ON CONFLICT DO NOTHING;
