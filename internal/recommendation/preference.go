package recommendation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	DemoUserIDHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	PreferenceTTL  = 15 * time.Minute
)

var ErrInvalidUserIDHash = errors.New("invalid user id hash")

type Preference struct {
	UserIDHash           string    `json:"user_id_hash"`
	PredictedDestination string    `json:"predicted_destination"`
	PreferredCategories  []string  `json:"preferred_categories"`
	BudgetMinPoints      int64     `json:"budget_min_points"`
	BudgetMaxPoints      int64     `json:"budget_max_points"`
	GeneratedAt          time.Time `json:"generated_at"`
	ExpiresAt            time.Time `json:"expires_at"`
	Source               string    `json:"-"`
}

type PreferenceStore struct {
	client *redis.Client
	ttl    time.Duration
	db     *sql.DB
}

func NewPreferenceStore(client *redis.Client, ttl time.Duration) *PreferenceStore {
	if ttl <= 0 {
		ttl = PreferenceTTL
	}
	return &PreferenceStore{client: client, ttl: ttl}
}

func (s *PreferenceStore) WithDB(db *sql.DB) *PreferenceStore {
	if s != nil {
		s.db = db
	}
	return s
}

func DemoPreference(userIDHash string) Preference {
	now := time.Now().UTC()
	preference := Preference{
		UserIDHash:           userIDHash,
		PredictedDestination: "R04",
		PreferredCategories:  []string{"coffee", "lunch"},
		BudgetMinPoints:      80,
		BudgetMaxPoints:      300,
		GeneratedAt:          now,
		ExpiresAt:            now.Add(PreferenceTTL),
		Source:               "fallback",
	}
	if userIDHash != DemoUserIDHash {
		preference.PredictedDestination = ""
		preference.PreferredCategories = []string{"general"}
	}
	return preference
}

func (s *PreferenceStore) Get(ctx context.Context, userIDHash string) (Preference, error) {
	if userIDHash == "" {
		return Preference{}, ErrInvalidUserIDHash
	}
	fallback := DemoPreference(userIDHash)
	if s == nil {
		return fallback, nil
	}

	if s.client != nil {
		encoded, err := s.client.Get(ctx, preferenceKey(userIDHash)).Bytes()
		if err == nil {
			var preference Preference
			if json.Unmarshal(encoded, &preference) == nil && preference.UserIDHash == userIDHash {
				preference.Source = "redis"
				return preference, nil
			}
		} else if !errors.Is(err, redis.Nil) {
			// Continue to PostgreSQL so a temporary cache outage does not hide saved preferences.
		}
	}

	if s.db != nil {
		var stationsJSON, categoriesJSON []byte
		var predictedDestination, timezone string
		var budgetMin, budgetMax int64
		if err := s.db.QueryRowContext(ctx, `
			SELECT favorite_station_ids, preferred_categories, budget_min_points, budget_max_points, timezone
			FROM users WHERE user_id_hash = $1`, userIDHash).Scan(
			&stationsJSON, &categoriesJSON, &budgetMin, &budgetMax, &timezone); err == nil {
			var stations, categories []string
			if json.Unmarshal(stationsJSON, &stations) == nil && json.Unmarshal(categoriesJSON, &categories) == nil {
				if len(stations) > 0 {
					predictedDestination = stations[0]
				}
				now := time.Now().UTC()
				preference := Preference{
					UserIDHash:           userIDHash,
					PredictedDestination: predictedDestination,
					PreferredCategories:  categories,
					BudgetMinPoints:      budgetMin,
					BudgetMaxPoints:      budgetMax,
					GeneratedAt:          now,
					ExpiresAt:            now.Add(s.ttl),
					Source:               "postgres",
				}
				if encoded, err := json.Marshal(preference); err == nil && s.client != nil {
					_ = s.client.Set(ctx, preferenceKey(userIDHash), encoded, s.ttl).Err()
				}
				return preference, nil
			}
		}
	}

	fallback.ExpiresAt = fallback.GeneratedAt.Add(s.ttl)
	if encoded, err := json.Marshal(fallback); err == nil && s.client != nil {
		_ = s.client.Set(ctx, preferenceKey(userIDHash), encoded, s.ttl).Err()
	}
	return fallback, nil
}

func preferenceKey(userIDHash string) string {
	return "preference:" + userIDHash
}
