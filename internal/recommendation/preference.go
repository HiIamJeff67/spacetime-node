package recommendation

import (
	"context"
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
}

func NewPreferenceStore(client *redis.Client, ttl time.Duration) *PreferenceStore {
	if ttl <= 0 {
		ttl = PreferenceTTL
	}
	return &PreferenceStore{client: client, ttl: ttl}
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
	if s == nil || s.client == nil {
		return fallback, nil
	}

	encoded, err := s.client.Get(ctx, preferenceKey(userIDHash)).Bytes()
	if err == nil {
		var preference Preference
		if json.Unmarshal(encoded, &preference) == nil && preference.UserIDHash == userIDHash {
			preference.Source = "redis"
			return preference, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		return fallback, nil
	}

	fallback.ExpiresAt = fallback.GeneratedAt.Add(s.ttl)
	if encoded, err := json.Marshal(fallback); err == nil {
		_ = s.client.Set(ctx, preferenceKey(userIDHash), encoded, s.ttl).Err()
	}
	return fallback, nil
}

func preferenceKey(userIDHash string) string {
	return "preference:" + userIDHash
}
