package notification

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidSubscription  = errors.New("invalid notification subscription")
	ErrSubscriptionNotFound = errors.New("notification subscription not found")
	ErrUserNotFound         = errors.New("user not found")
)

type Service struct {
	db *sql.DB
}

type Subscription struct {
	ID      string
	Active  bool
	Channel string
}

type UserNotificationSettings struct {
	IDHash     string
	Timezone   string
	Enabled    bool
	StartLocal string
	EndLocal   string
}

func NewService(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Register(ctx context.Context, userIDHash, endpoint, p256dh, auth, userAgent string) (Subscription, error) {
	if s == nil || s.db == nil || !validUserIDHash(userIDHash) || !validEndpoint(endpoint) || strings.TrimSpace(p256dh) == "" || strings.TrimSpace(auth) == "" {
		return Subscription{}, ErrInvalidSubscription
	}
	id := uuid.NewString()
	var subscription Subscription
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO notification_subscriptions
		  (subscription_id, user_id, endpoint, p256dh, auth, user_agent, active, updated_at)
		SELECT $1, user_id, $2, $3, $4, NULLIF($5, ''), true, now()
		FROM users WHERE user_id_hash = $6
		ON CONFLICT (user_id, endpoint) DO UPDATE
		SET p256dh = EXCLUDED.p256dh,
		    auth = EXCLUDED.auth,
		    user_agent = EXCLUDED.user_agent,
		    active = true,
		    updated_at = now()
		RETURNING subscription_id, active`, id, endpoint, p256dh, auth, strings.TrimSpace(userAgent), userIDHash).Scan(&subscription.ID, &subscription.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrUserNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	subscription.Channel = "demo"
	return subscription, nil
}

func (s *Service) Revoke(ctx context.Context, userIDHash, subscriptionID string) (Subscription, error) {
	if s == nil || s.db == nil || !validUserIDHash(userIDHash) || strings.TrimSpace(subscriptionID) == "" {
		return Subscription{}, ErrInvalidSubscription
	}
	var subscription Subscription
	err := s.db.QueryRowContext(ctx, `
		UPDATE notification_subscriptions ns
		SET active = false, updated_at = now()
		FROM users u
		WHERE ns.subscription_id = $1 AND ns.user_id = u.user_id AND u.user_id_hash = $2
		RETURNING ns.subscription_id, ns.active`, subscriptionID, userIDHash).Scan(&subscription.ID, &subscription.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return Subscription{}, ErrSubscriptionNotFound
	}
	if err != nil {
		return Subscription{}, err
	}
	subscription.Channel = "demo"
	return subscription, nil
}

func LoadUserSettings(ctx context.Context, tx *sql.Tx, userIDHash string) (UserNotificationSettings, string, error) {
	var settings UserNotificationSettings
	var userID string
	var start, end sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT user_id::text, user_id_hash, timezone, notifications_enabled,
		       notification_start_local, notification_end_local
		FROM users WHERE user_id_hash = $1`, userIDHash).Scan(
		&userID, &settings.IDHash, &settings.Timezone, &settings.Enabled, &start, &end)
	if errors.Is(err, sql.ErrNoRows) {
		return UserNotificationSettings{}, "", ErrUserNotFound
	}
	if err != nil {
		return UserNotificationSettings{}, "", err
	}
	settings.StartLocal, settings.EndLocal = start.String, end.String
	return settings, userID, nil
}

func WithinNotificationWindow(now time.Time, timezone, startLocal, endLocal string) bool {
	if startLocal == "" || endLocal == "" {
		return false
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false
	}
	start, err := time.Parse("15:04", startLocal)
	if err != nil {
		return false
	}
	end, err := time.Parse("15:04", endLocal)
	if err != nil {
		return false
	}
	local := now.In(location)
	minutes := local.Hour()*60 + local.Minute()
	startMinutes := start.Hour()*60 + start.Minute()
	endMinutes := end.Hour()*60 + end.Minute()
	if startMinutes == endMinutes {
		return true
	}
	if startMinutes < endMinutes {
		return minutes >= startMinutes && minutes < endMinutes
	}
	return minutes >= startMinutes || minutes < endMinutes
}

func validEndpoint(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "https://") && len(value) <= 2048
}

func validUserIDHash(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func requiredWindow(settings UserNotificationSettings) error {
	if !settings.Enabled {
		return nil
	}
	if settings.Timezone == "" || settings.StartLocal == "" || settings.EndLocal == "" {
		return fmt.Errorf("notification window is not configured")
	}
	return nil
}
