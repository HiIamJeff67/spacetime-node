CREATE TABLE notification_subscriptions (
    subscription_id TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(user_id),
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    user_agent TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, endpoint)
);

CREATE TABLE notification_deliveries (
    notification_id TEXT PRIMARY KEY,
    subscription_id TEXT NOT NULL REFERENCES notification_subscriptions(subscription_id),
    user_id UUID NOT NULL REFERENCES users(user_id),
    journey_id TEXT NOT NULL REFERENCES journeys(journey_id),
    recommendation_id TEXT NOT NULL REFERENCES recommendations(recommendation_id),
    status TEXT NOT NULL CHECK (status IN ('sent', 'delivered', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 1 CHECK (attempts > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (subscription_id, recommendation_id)
);

CREATE INDEX notification_subscriptions_user_active_idx
    ON notification_subscriptions (user_id, active);
