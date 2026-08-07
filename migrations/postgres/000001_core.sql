CREATE TABLE users (
    user_id UUID PRIMARY KEY,
    user_id_hash TEXT NOT NULL UNIQUE CHECK (user_id_hash ~ '^sha256:[0-9a-f]{64}$'),
    display_name TEXT NOT NULL,
    point_balance BIGINT NOT NULL DEFAULT 0 CHECK (point_balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE stations (
    station_id TEXT PRIMARY KEY,
    name_zh TEXT NOT NULL,
    line_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE offers (
    offer_id TEXT PRIMARY KEY,
    merchant_id TEXT NOT NULL,
    station_id TEXT NOT NULL REFERENCES stations(station_id),
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    points_cost BIGINT NOT NULL CHECK (points_cost >= 0),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL CHECK (ends_at > starts_at),
    is_active BOOLEAN NOT NULL DEFAULT true,
    content_version BIGINT NOT NULL DEFAULT 1 CHECK (content_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE inventory (
    offer_id TEXT PRIMARY KEY REFERENCES offers(offer_id),
    available_quantity INTEGER NOT NULL CHECK (available_quantity >= 0),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE journeys (
    journey_id TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(user_id),
    station_id TEXT NOT NULL REFERENCES stations(station_id),
    line_id TEXT NOT NULL,
    position_id TEXT,
    entered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE recommendations (
    recommendation_id TEXT PRIMARY KEY,
    journey_id TEXT NOT NULL REFERENCES journeys(journey_id),
    offer_id TEXT NOT NULL REFERENCES offers(offer_id),
    score NUMERIC(8, 4) NOT NULL,
    reasons JSONB NOT NULL CHECK (jsonb_typeof(reasons) = 'array'),
    copy_title TEXT,
    copy_body TEXT,
    copy_tone TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE redemptions (
    redemption_id TEXT PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(user_id),
    journey_id TEXT REFERENCES journeys(journey_id),
    offer_id TEXT NOT NULL REFERENCES offers(offer_id),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'rejected', 'verified')),
    points_cost BIGINT NOT NULL CHECK (points_cost >= 0),
    merchant_verification_code TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, idempotency_key)
);

CREATE TABLE points_ledger (
    entry_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(user_id),
    redemption_id TEXT REFERENCES redemptions(redemption_id),
    delta BIGINT NOT NULL CHECK (delta <> 0),
    balance_after BIGINT NOT NULL CHECK (balance_after >= 0),
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    event_id TEXT PRIMARY KEY,
    topic TEXT NOT NULL,
    event_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    last_error TEXT
);

CREATE TABLE processed_events (
    consumer_name TEXT NOT NULL,
    event_id TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_name, event_id)
);

CREATE INDEX recommendations_journey_created_at_idx ON recommendations (journey_id, created_at DESC);
CREATE INDEX redemptions_user_created_at_idx ON redemptions (user_id, created_at DESC);
CREATE INDEX outbox_events_pending_idx ON outbox_events (occurred_at) WHERE published_at IS NULL;
