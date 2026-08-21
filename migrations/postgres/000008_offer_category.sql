ALTER TABLE offers
    ADD COLUMN IF NOT EXISTS category TEXT NOT NULL DEFAULT 'general'
        CHECK (length(trim(category)) > 0);

UPDATE offers
SET category = CASE offer_id
    WHEN 'offer-coffee-xinyi' THEN 'coffee'
    WHEN 'offer-lunch-xinyi' THEN 'lunch'
    WHEN 'offer-dessert-101' THEN 'dessert'
    ELSE 'general'
END;
