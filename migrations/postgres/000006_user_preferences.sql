ALTER TABLE users
    ADD COLUMN favorite_station_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN preferred_categories JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN budget_min_points BIGINT NOT NULL DEFAULT 0 CHECK (budget_min_points >= 0),
    ADD COLUMN budget_max_points BIGINT NOT NULL DEFAULT 0 CHECK (budget_max_points >= budget_min_points),
    ADD COLUMN timezone TEXT NOT NULL DEFAULT 'Asia/Taipei',
    ADD COLUMN notifications_enabled BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN notification_start_local TEXT,
    ADD COLUMN notification_end_local TEXT;

ALTER TABLE users
    ADD CONSTRAINT users_favorite_station_ids_array CHECK (jsonb_typeof(favorite_station_ids) = 'array'),
    ADD CONSTRAINT users_preferred_categories_array CHECK (jsonb_typeof(preferred_categories) = 'array');

UPDATE users
SET favorite_station_ids = '["R04"]'::jsonb,
    preferred_categories = '["coffee", "lunch"]'::jsonb,
    budget_min_points = 80,
    budget_max_points = 300
WHERE user_id_hash = concat('sha256:', repeat('a', 64))
  AND budget_min_points = 0
  AND budget_max_points = 0;
