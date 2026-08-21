CREATE TABLE user_preference_weights (
    user_id UUID NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    category TEXT NOT NULL CHECK (length(trim(category)) > 0),
    weight NUMERIC(8, 4) NOT NULL DEFAULT 0 CHECK (weight >= -5 AND weight <= 5),
    click_count INTEGER NOT NULL DEFAULT 0 CHECK (click_count >= 0),
    dismiss_count INTEGER NOT NULL DEFAULT 0 CHECK (dismiss_count >= 0),
    redemption_count INTEGER NOT NULL DEFAULT 0 CHECK (redemption_count >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, category)
);

CREATE INDEX user_preference_weights_user_idx ON user_preference_weights (user_id);
