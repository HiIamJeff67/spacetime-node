-- SCRUM-47: keep the externally selectable demo stations registered on existing volumes.
ALTER TABLE offers
    ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'general'
        CHECK (length(trim(category)) > 0);

INSERT INTO stations (station_id, name_zh, line_id)
VALUES
    ('R04', '信義安和', 'R'),
    ('R03', '台北101／世貿', 'R'),
    ('BL12', '市政府', 'BL')
ON CONFLICT (station_id) DO UPDATE
SET name_zh = EXCLUDED.name_zh,
    line_id = EXCLUDED.line_id;

-- Ensure the second selectable demo station has an active offer after an upgrade
-- from an older database volume that only contained the original R04 seed.
INSERT INTO offers (
    offer_id, merchant_id, station_id, title, description, points_cost,
    starts_at, ends_at, category
)
VALUES (
    'offer-dessert-101',
    'merchant-dessert-demo',
    'R03',
    '甜點兌換券',
    '台北101／世貿站附近甜點兌換。',
    150,
    now() - interval '1 day',
    now() + interval '30 days',
    'dessert'
)
ON CONFLICT (offer_id) DO UPDATE
SET station_id = EXCLUDED.station_id,
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    points_cost = EXCLUDED.points_cost,
    category = EXCLUDED.category,
    is_active = true,
    starts_at = EXCLUDED.starts_at,
    ends_at = EXCLUDED.ends_at;

INSERT INTO inventory (offer_id, available_quantity)
VALUES ('offer-dessert-101', 20)
ON CONFLICT (offer_id) DO NOTHING;
