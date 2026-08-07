INSERT INTO users (user_id, user_id_hash, display_name, point_balance)
VALUES ('00000000-0000-0000-0000-000000000001', concat('sha256:', repeat('a', 64)), 'Demo Rider', 1200)
ON CONFLICT (user_id) DO NOTHING;

INSERT INTO stations (station_id, name_zh, line_id)
VALUES
    ('R04', '信義安和', 'R'),
    ('R03', '台北101／世貿', 'R'),
    ('BL12', '市政府', 'BL')
ON CONFLICT (station_id) DO NOTHING;

INSERT INTO offers (offer_id, merchant_id, station_id, title, description, points_cost, starts_at, ends_at)
VALUES
    ('offer-coffee-xinyi', 'merchant-coffee-demo', 'R04', '通勤咖啡折抵 50 元', '信義安和站附近咖啡折抵優惠。', 80, now() - interval '1 day', now() + interval '30 days'),
    ('offer-lunch-xinyi', 'merchant-lunch-demo', 'R04', '午間套餐折抵 80 元', '適合午餐時段的限定折抵優惠。', 120, now() - interval '1 day', now() + interval '30 days'),
    ('offer-dessert-101', 'merchant-dessert-demo', 'R03', '甜點兌換券', '台北101／世貿站附近甜點兌換。', 150, now() - interval '1 day', now() + interval '30 days')
ON CONFLICT (offer_id) DO NOTHING;

INSERT INTO inventory (offer_id, available_quantity)
VALUES
    ('offer-coffee-xinyi', 50),
    ('offer-lunch-xinyi', 30),
    ('offer-dessert-101', 20)
ON CONFLICT (offer_id) DO NOTHING;

INSERT INTO points_ledger (user_id, delta, balance_after, reason)
SELECT '00000000-0000-0000-0000-000000000001', 1200, 1200, 'demo_initial_balance'
WHERE NOT EXISTS (
    SELECT 1
    FROM points_ledger
    WHERE user_id = '00000000-0000-0000-0000-000000000001'
      AND reason = 'demo_initial_balance'
);

INSERT INTO journeys (journey_id, user_id, station_id, line_id, position_id)
VALUES ('journey-demo-001', '00000000-0000-0000-0000-000000000001', 'R04', 'R', 'exit-3')
ON CONFLICT (journey_id) DO NOTHING;

INSERT INTO recommendations (recommendation_id, journey_id, offer_id, score, reasons, copy_title, copy_body, copy_tone)
VALUES (
    'recommendation-demo-001',
    'journey-demo-001',
    'offer-coffee-xinyi',
    0.8600,
    '["near_station", "point_roi"]'::jsonb,
    '信義安和站的通勤小確幸',
    '用 80 點折抵附近咖啡，為通勤補充能量。',
    'friendly'
)
ON CONFLICT (recommendation_id) DO NOTHING;
