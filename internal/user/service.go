package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
)

var (
	ErrInvalidRequest = errors.New("invalid user request")
	ErrUserNotFound   = errors.New("user not found")
)

type Service struct {
	v1.UnimplementedUserServiceServer
	db *sql.DB
}

const anonymousDemoInitialPoints int64 = 1200

type contextExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// EnsureDemoUser provisions an anonymous browser-session user on first use.
// ponytail: demo-only auto-provisioning; replace with authenticated identity when login exists.
func EnsureDemoUser(ctx context.Context, execer contextExecer, userIDHash string) error {
	if execer == nil || !validUserIDHash(userIDHash) {
		return ErrInvalidRequest
	}
	userID := uuid.NewString()
	_, err := execer.ExecContext(ctx, `
		WITH created AS (
			INSERT INTO users (user_id, user_id_hash, display_name, point_balance)
			VALUES ($1, $2, 'Demo Rider', $3)
			ON CONFLICT (user_id_hash) DO NOTHING
			RETURNING user_id
		)
		INSERT INTO points_ledger (user_id, delta, balance_after, reason)
		SELECT user_id, $3, $3, 'demo_initial_balance'
		FROM created`, userID, userIDHash, anonymousDemoInitialPoints)
	return err
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) GetUserProfile(ctx context.Context, request *v1.GetUserProfileRequest) (*v1.GetUserProfileResponse, error) {
	if s == nil || s.db == nil || request == nil || !validUserIDHash(request.GetUserIdHash()) {
		return nil, v1.ErrorInvalidRequest("user_id_hash must be a sha256 hash")
	}
	if err := EnsureDemoUser(ctx, s.db, request.GetUserIdHash()); err != nil {
		return nil, err
	}
	profile, err := s.profile(ctx, request.GetUserIdHash())
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.GetUserProfileResponse{Profile: profile}, nil
}

func (s *Service) UpdateUserPreferences(ctx context.Context, request *v1.UpdateUserPreferencesRequest) (*v1.UpdateUserPreferencesResponse, error) {
	if s == nil || s.db == nil || request == nil || !validUserIDHash(request.GetUserIdHash()) {
		return nil, v1.ErrorInvalidRequest("user_id_hash must be a sha256 hash")
	}
	if err := validatePreferences(request); err != nil {
		return nil, v1.ErrorInvalidRequest("%s", err)
	}
	if err := EnsureDemoUser(ctx, s.db, request.GetUserIdHash()); err != nil {
		return nil, err
	}
	stations, err := json.Marshal(request.GetFavoriteStationIds())
	if err != nil {
		return nil, err
	}
	categories, err := json.Marshal(request.GetPreferredCategories())
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET favorite_station_ids = $2,
		    preferred_categories = $3,
		    budget_min_points = $4,
		    budget_max_points = $5,
		    timezone = $6,
		    notifications_enabled = $7,
		    notification_start_local = NULLIF($8, ''),
		    notification_end_local = NULLIF($9, ''),
		    updated_at = now()
		WHERE user_id_hash = $1`,
		request.GetUserIdHash(), stations, categories, request.GetBudgetMinPoints(), request.GetBudgetMaxPoints(),
		request.GetTimezone(), request.GetNotificationsEnabled(), request.GetNotificationStartLocal(), request.GetNotificationEndLocal())
	if err != nil {
		return nil, err
	}
	if count, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if count != 1 {
		return nil, ErrUserNotFound
	}
	profile, err := s.profile(ctx, request.GetUserIdHash())
	if err != nil {
		return nil, mapError(err)
	}
	return &v1.UpdateUserPreferencesResponse{Profile: profile}, nil
}

func (s *Service) profile(ctx context.Context, userIDHash string) (*v1.UserProfile, error) {
	var (
		profile                      v1.UserProfile
		createdAt, updatedAt         time.Time
		stationsJSON, categoriesJSON []byte
		startLocal, endLocal         sql.NullString
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id::text, user_id_hash, display_name, created_at, updated_at,
		       favorite_station_ids, preferred_categories, budget_min_points,
		       budget_max_points, timezone, notifications_enabled,
		       notification_start_local, notification_end_local
		FROM users
		WHERE user_id_hash = $1`, userIDHash).Scan(
		&profile.UserId, &profile.UserIdHash, &profile.DisplayName, &createdAt, &updatedAt,
		&stationsJSON, &categoriesJSON, &profile.BudgetMinPoints, &profile.BudgetMaxPoints,
		&profile.Timezone, &profile.NotificationsEnabled, &startLocal, &endLocal)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(stationsJSON, &profile.FavoriteStationIds); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(categoriesJSON, &profile.PreferredCategories); err != nil {
		return nil, err
	}
	profile.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	profile.UpdatedAt = updatedAt.UTC().Format(time.RFC3339Nano)
	profile.NotificationStartLocal = startLocal.String
	profile.NotificationEndLocal = endLocal.String
	return &profile, nil
}

func validatePreferences(request *v1.UpdateUserPreferencesRequest) error {
	if request.GetBudgetMinPoints() < 0 || request.GetBudgetMaxPoints() < request.GetBudgetMinPoints() {
		return errors.New("budget range is invalid")
	}
	if request.GetTimezone() == "" {
		return errors.New("timezone is required")
	}
	if _, err := time.LoadLocation(request.GetTimezone()); err != nil {
		return errors.New("timezone is invalid")
	}
	if len(request.GetFavoriteStationIds()) > 20 || len(request.GetPreferredCategories()) > 20 {
		return errors.New("preference list is too long")
	}
	for _, value := range append(append([]string{}, request.GetFavoriteStationIds()...), request.GetPreferredCategories()...) {
		if strings.TrimSpace(value) == "" || len(value) > 80 {
			return errors.New("preference values are invalid")
		}
	}
	start, end := request.GetNotificationStartLocal(), request.GetNotificationEndLocal()
	if (start == "") != (end == "") {
		return errors.New("notification window must include both start and end")
	}
	for _, value := range []string{start, end} {
		if value != "" {
			if _, err := time.Parse("15:04", value); err != nil {
				return errors.New("notification time must use HH:MM")
			}
		}
	}
	if request.GetNotificationsEnabled() && start == "" {
		return errors.New("notification window is required when notifications are enabled")
	}
	return nil
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

func mapError(err error) error {
	switch {
	case errors.Is(err, ErrUserNotFound):
		return v1.ErrorUserNotFound("user not found")
	default:
		return err
	}
}
