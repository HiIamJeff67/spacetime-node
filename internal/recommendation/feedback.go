package recommendation

import (
	"context"
	"database/sql"
	"errors"
)

const (
	RecommendationDismissedTopic = "recommendation.dismissed.v1"
	RedemptionSucceededTopic     = "redemption.succeeded.v1"
)

var ErrInvalidFeedbackEvent = errors.New("invalid recommendation feedback event")

// ApplyPreferenceFeedback records a bounded category signal in the same
// transaction as its source event, so a successful event cannot lose its
// personalization update.
func ApplyPreferenceFeedback(ctx context.Context, tx *sql.Tx, userIDHash, offerID, eventType string) error {
	if tx == nil || userIDHash == "" || offerID == "" {
		return ErrInvalidFeedbackEvent
	}

	weight, clicks, dismisses, redemptions := feedbackDelta(eventType)
	if weight == 0 && clicks == 0 && dismisses == 0 && redemptions == 0 {
		return nil
	}

	var userID, category string
	if err := tx.QueryRowContext(ctx, `
		SELECT u.user_id, o.category
		FROM users u
		JOIN offers o ON o.offer_id = $2
		WHERE u.user_id_hash = $1`, userIDHash, offerID).Scan(&userID, &category); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO user_preference_weights (
			user_id, category, weight, click_count, dismiss_count, redemption_count
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, category) DO UPDATE SET
			weight = LEAST(5, GREATEST(-5, user_preference_weights.weight + EXCLUDED.weight)),
			click_count = user_preference_weights.click_count + EXCLUDED.click_count,
			dismiss_count = user_preference_weights.dismiss_count + EXCLUDED.dismiss_count,
			redemption_count = user_preference_weights.redemption_count + EXCLUDED.redemption_count,
			updated_at = now()`, userID, category, weight, clicks, dismisses, redemptions)
	return err
}

func feedbackDelta(eventType string) (float64, int, int, int) {
	switch eventType {
	case "recommendation.clicked.v1":
		return 0.5, 1, 0, 0
	case RecommendationDismissedTopic:
		return -0.75, 0, 1, 0
	case RedemptionSucceededTopic:
		return 1.5, 0, 0, 1
	default:
		return 0, 0, 0, 0
	}
}

func LoadPreferenceWeights(ctx context.Context, db *sql.DB, userIDHash string) (map[string]float64, error) {
	weights := make(map[string]float64)
	if db == nil || userIDHash == "" {
		return weights, nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT w.category, w.weight
		FROM user_preference_weights w
		JOIN users u ON u.user_id = w.user_id
		WHERE u.user_id_hash = $1`, userIDHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var category string
		var weight float64
		if err := rows.Scan(&category, &weight); err != nil {
			return nil, err
		}
		weights[category] = weight
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return weights, nil
}
