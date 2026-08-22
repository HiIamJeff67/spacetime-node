-- Each user can redeem a given offer only once.
CREATE UNIQUE INDEX IF NOT EXISTS redemptions_user_offer_once_idx
ON redemptions (user_id, offer_id)
WHERE status IN ('succeeded', 'verified');
