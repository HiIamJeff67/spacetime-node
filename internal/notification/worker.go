package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"spacetime-node/internal/platform/observability"
	"spacetime-node/internal/platform/outbox"
)

const (
	RecommendationTopic  = "recommendation.created.v1"
	NotificationConsumer = "notification-worker-v1"
)

type RecommendationCreatedEvent struct {
	EventID          string `json:"event_id"`
	EventType        string `json:"event_type"`
	SchemaVersion    int    `json:"schema_version"`
	OccurredAt       string `json:"occurred_at"`
	Producer         string `json:"producer"`
	TraceID          string `json:"trace_id"`
	JourneyID        string `json:"journey_id"`
	RecommendationID string `json:"recommendation_id"`
	UserIDHash       string `json:"user_id_hash"`
	Payload          struct {
		RecommendationID string   `json:"recommendation_id"`
		OfferID          string   `json:"offer_id"`
		Reasons          []string `json:"reasons"`
		CopySource       string   `json:"copy_source"`
	} `json:"payload"`
}

func DecodeRecommendationCreatedEvent(data []byte) (RecommendationCreatedEvent, error) {
	var event RecommendationCreatedEvent
	if json.Unmarshal(data, &event) != nil || event.EventID == "" || event.EventType != RecommendationTopic || event.SchemaVersion != 1 || event.TraceID == "" || event.JourneyID == "" || event.RecommendationID == "" || event.UserIDHash == "" || event.Payload.RecommendationID == "" || event.Payload.OfferID == "" {
		return RecommendationCreatedEvent{}, errors.New("invalid recommendation created event")
	}
	return event, nil
}

func Run(ctx context.Context, brokers []string, db *sql.DB, logger *log.Logger) error {
	if len(brokers) == 0 || db == nil {
		return errors.New("notification worker dependencies are missing")
	}
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: RecommendationTopic, GroupID: NotificationConsumer, MinBytes: 1, MaxBytes: 10e6})
	defer reader.Close()
	if logger == nil {
		logger = log.Default()
	}
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		eventCtx := observability.ExtractKafka(ctx, message.Headers)
		eventCtx, span := observability.StartKafkaSpan(eventCtx, RecommendationTopic, "", true)
		event, err := DecodeRecommendationCreatedEvent(message.Value)
		if err != nil {
			span.RecordError(err)
			span.End()
			if err := reader.CommitMessages(eventCtx, message); err != nil {
				return err
			}
			continue
		}
		_, err = outbox.Process(eventCtx, db, NotificationConsumer, event.EventID, func(ctx context.Context, tx *sql.Tx) error {
			return deliver(ctx, tx, event)
		})
		if err != nil {
			span.RecordError(err)
			span.End()
			return err
		}
		if err := reader.CommitMessages(eventCtx, message); err != nil {
			span.RecordError(err)
			span.End()
			return err
		}
		span.End()
	}
}

func deliver(ctx context.Context, tx *sql.Tx, event RecommendationCreatedEvent) error {
	settings, userID, err := LoadUserSettings(ctx, tx, event.UserIDHash)
	if err != nil {
		return err
	}
	if !settings.Enabled || requiredWindow(settings) != nil || !WithinNotificationWindow(time.Now(), settings.Timezone, settings.StartLocal, settings.EndLocal) {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT subscription_id FROM notification_subscriptions WHERE user_id = $1 AND active = true ORDER BY created_at`, userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var subscriptionID string
		if err := rows.Scan(&subscriptionID); err != nil {
			return err
		}
		notificationID := uuid.NewString()
		result, err := tx.ExecContext(ctx, `
			INSERT INTO notification_deliveries (notification_id, subscription_id, user_id, journey_id, recommendation_id, status, attempts)
			VALUES ($1, $2, $3, $4, $5, 'delivered', 1)
			ON CONFLICT (subscription_id, recommendation_id) DO NOTHING`, notificationID, subscriptionID, userID, event.JourneyID, event.RecommendationID)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			continue
		}
		for _, eventType := range []string{"notification.sent.v1", "notification.delivered.v1"} {
			eventID := uuid.NewString()
			payload, err := json.Marshal(map[string]any{
				"event_id":          eventID,
				"event_type":        eventType,
				"schema_version":    1,
				"occurred_at":       time.Now().UTC().Format(time.RFC3339Nano),
				"producer":          "notification-worker",
				"trace_id":          event.TraceID,
				"journey_id":        event.JourneyID,
				"recommendation_id": event.RecommendationID,
				"user_id_hash":      event.UserIDHash,
				"payload":           map[string]string{"notification_id": notificationID, "channel": "demo", "provider": "deterministic-mock"},
			})
			if err != nil {
				return err
			}
			if err := outbox.Enqueue(ctx, tx, outbox.Event{ID: eventID, Topic: eventType, Key: event.UserIDHash, Payload: payload}); err != nil {
				return err
			}
		}
	}
	return rows.Err()
}
