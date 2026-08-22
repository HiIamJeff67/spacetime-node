-- Demo catalog expansion: add realistic offers for more selectable stations.
-- Keep this additive and safe to re-run on an existing demo volume.

INSERT INTO stations (station_id, name_zh, line_id)
VALUES
    ('R04', '信義安和', 'R'),
    ('R03', '台北101／世貿', 'R'),
    ('R09', '台北車站', 'R'),
    ('G12', '西門', 'G'),
    ('G07', '公館', 'G'),
    ('BL18', '市政府', 'BL'),
    ('BL12', '台北車站', 'BL'),
    ('O09', '行天宮', 'O')
ON CONFLICT (station_id) DO UPDATE
SET name_zh = EXCLUDED.name_zh,
    line_id = EXCLUDED.line_id;

INSERT INTO offers (
    offer_id, merchant_id, station_id, title, description, points_cost,
    starts_at, ends_at, category, content_version, is_active, updated_at
)
VALUES
    ('offer-dessert-xinyi', 'merchant-dessert-xinyi', 'R04', '信義安和甜點折抵 60 元', '信義安和站附近甜點店折抵優惠，適合下班後的小點心。', 100, now() - interval '1 day', now() + interval '30 days', 'dessert', 1, true, now()),
    ('offer-coffee-101', 'merchant-coffee-101', 'R03', '台北101 精品咖啡折抵 50 元', '台北101／世貿站附近咖啡店折抵優惠。', 90, now() - interval '1 day', now() + interval '30 days', 'coffee', 1, true, now()),
    ('offer-drink-101', 'merchant-drink-101', 'R03', '台北101 手搖飲折抵 30 元', '台北101／世貿站附近飲品折抵優惠。', 70, now() - interval '1 day', now() + interval '30 days', 'drink', 1, true, now()),
    ('offer-breakfast-taipei', 'merchant-breakfast-taipei', 'R09', '台北車站早餐咖啡折抵 50 元', '台北車站周邊早餐與咖啡折抵優惠。', 110, now() - interval '1 day', now() + interval '30 days', 'coffee', 1, true, now()),
    ('offer-fastfood-taipei', 'merchant-fastfood-taipei', 'R09', '台北車站速食套餐折抵 100 元', '台北車站附近連鎖速食套餐折抵優惠。', 180, now() - interval '1 day', now() + interval '30 days', 'fast food', 1, true, now()),
    ('offer-burger-ximen', 'merchant-burger-ximen', 'G12', '西門町漢堡套餐折抵 100 元', '西門站附近漢堡套餐折抵優惠。', 160, now() - interval '1 day', now() + interval '30 days', 'fast food', 1, true, now()),
    ('offer-drink-ximen', 'merchant-drink-ximen', 'G12', '西門町手搖飲折抵 30 元', '西門站附近手搖飲折抵優惠。', 75, now() - interval '1 day', now() + interval '30 days', 'drink', 1, true, now()),
    ('offer-coffee-gongguan', 'merchant-coffee-gongguan', 'G07', '公館咖啡折抵 50 元', '公館站附近咖啡店折抵優惠。', 90, now() - interval '1 day', now() + interval '30 days', 'coffee', 1, true, now()),
    ('offer-lunch-gongguan', 'merchant-lunch-gongguan', 'G07', '公館學生午餐折抵 80 元', '公館商圈午餐套餐折抵優惠。', 140, now() - interval '1 day', now() + interval '30 days', 'lunch', 1, true, now()),
    ('offer-convenience-cityhall', 'merchant-convenience-cityhall', 'BL18', '市政府便利商店折抵 50 元', '市政府站附近便利商店消費折抵優惠。', 100, now() - interval '1 day', now() + interval '30 days', 'convenience store', 1, true, now()),
    ('offer-life-cityhall', 'merchant-life-cityhall', 'BL18', '市政府生活百貨折抵 80 元', '市政府站附近生活百貨消費折抵優惠。', 130, now() - interval '1 day', now() + interval '30 days', 'local', 1, true, now()),
    ('offer-convenience-taipei', 'merchant-convenience-taipei', 'BL12', '台北車站便利商店折抵 50 元', '台北車站周邊便利商店消費折抵優惠。', 100, now() - interval '1 day', now() + interval '30 days', 'convenience store', 1, true, now()),
    ('offer-lunch-xingtian', 'merchant-lunch-xingtian', 'O09', '行天宮便當折抵 80 元', '行天宮站附近便當與午餐折抵優惠。', 150, now() - interval '1 day', now() + interval '30 days', 'lunch', 1, true, now()),
    ('offer-dessert-xingtian', 'merchant-dessert-xingtian', 'O09', '行天宮豆花折抵 40 元', '行天宮站附近甜品折抵優惠。', 90, now() - interval '1 day', now() + interval '30 days', 'dessert', 1, true, now())
ON CONFLICT (offer_id) DO UPDATE
SET merchant_id = EXCLUDED.merchant_id,
    station_id = EXCLUDED.station_id,
    title = EXCLUDED.title,
    description = EXCLUDED.description,
    points_cost = EXCLUDED.points_cost,
    starts_at = EXCLUDED.starts_at,
    ends_at = EXCLUDED.ends_at,
    category = EXCLUDED.category,
    content_version = GREATEST(offers.content_version, EXCLUDED.content_version),
    is_active = EXCLUDED.is_active,
    updated_at = now();

INSERT INTO inventory (offer_id, available_quantity)
SELECT offer_id, 25
FROM offers
WHERE offer_id IN (
    'offer-dessert-xinyi', 'offer-coffee-101', 'offer-drink-101',
    'offer-breakfast-taipei', 'offer-fastfood-taipei', 'offer-burger-ximen',
    'offer-drink-ximen', 'offer-coffee-gongguan', 'offer-lunch-gongguan',
    'offer-convenience-cityhall', 'offer-life-cityhall', 'offer-convenience-taipei',
    'offer-lunch-xingtian', 'offer-dessert-xingtian'
)
ON CONFLICT (offer_id) DO UPDATE
SET available_quantity = GREATEST(inventory.available_quantity, EXCLUDED.available_quantity),
    updated_at = now();
