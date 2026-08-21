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
	return RunWithProvider(ctx, brokers, db, deterministicProvider{}, logger)
}

func RunWithProvider(ctx context.Context, brokers []string, db *sql.DB, provider PushProvider, logger *log.Logger) error {
	if len(brokers) == 0 || db == nil {
		return errors.New("notification worker dependencies are missing")
	}
	if provider == nil {
		return ErrInvalidPushProvider
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
			return deliver(ctx, tx, event, provider)
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

func deliver(ctx context.Context, tx *sql.Tx, event RecommendationCreatedEvent, provider PushProvider) error {
	settings, userID, err := LoadUserSettings(ctx, tx, event.UserIDHash)
	if err != nil {
		return err
	}
	if !settings.Enabled || requiredWindow(settings) != nil || !WithinNotificationWindow(time.Now(), settings.Timezone, settings.StartLocal, settings.EndLocal) {
		return nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT ns.subscription_id, ns.endpoint, ns.p256dh, ns.auth, r.copy_title, r.copy_body
		FROM notification_subscriptions ns
		JOIN recommendations r ON r.recommendation_id = $2
		WHERE ns.user_id = $1 AND ns.active = true
		ORDER BY ns.created_at`, userID, event.RecommendationID)
	if err != nil {
		return err
	}
	type subscriptionDelivery struct {
		subscription PushSubscription
		title        string
		body         string
	}
	deliveries := make([]subscriptionDelivery, 0)
	for rows.Next() {
		var delivery subscriptionDelivery
		if err := rows.Scan(&delivery.subscription.ID, &delivery.subscription.Endpoint, &delivery.subscription.P256DH, &delivery.subscription.Auth, &delivery.title, &delivery.body); err != nil {
			rows.Close()
			return err
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, delivery := range deliveries {
		notificationID := uuid.NewString()
		var alreadyDelivered bool
		if err := tx.QueryRowContext(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM notification_deliveries
				WHERE subscription_id = $1 AND recommendation_id = $2
			)`, delivery.subscription.ID, event.RecommendationID).Scan(&alreadyDelivered); err != nil {
			return err
		}
		if alreadyDelivered {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"title": delivery.title,
			"body":  delivery.body,
			"data": map[string]string{
				"recommendation_id": event.RecommendationID,
				"offer_id":          event.Payload.OfferID,
			},
		})
		if err != nil {
			return err
		}
		status, err := provider.Send(ctx, delivery.subscription, payload)
		if err != nil {
			if IsInactiveSubscriptionError(err) {
				if _, updateErr := tx.ExecContext(ctx, `
					UPDATE notification_subscriptions
					SET active = false, updated_at = now()
					WHERE subscription_id = $1`, delivery.subscription.ID); updateErr != nil {
					return updateErr
				}
			}
			result, insertErr := tx.ExecContext(ctx, `
				INSERT INTO notification_deliveries (notification_id, subscription_id, user_id, journey_id, recommendation_id, status, attempts)
				VALUES ($1, $2, $3, $4, $5, 'failed', 1)
				ON CONFLICT (subscription_id, recommendation_id) DO NOTHING`, notificationID, delivery.subscription.ID, userID, event.JourneyID, event.RecommendationID)
			if insertErr != nil {
				return insertErr
			}
			inserted, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return rowsErr
			}
			if inserted > 0 {
				if err := enqueueNotificationEvent(ctx, tx, event, "notification.failed.v1", notificationID, "push", "webpush", PushFailureCode(err)); err != nil {
					return err
				}
			}
			continue
		}
		channel, providerName := "push", "webpush"
		if status == "delivered" {
			channel, providerName = "demo", "deterministic-mock"
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO notification_deliveries (notification_id, subscription_id, user_id, journey_id, recommendation_id, status, attempts)
			VALUES ($1, $2, $3, $4, $5, $6, 1)
			ON CONFLICT (subscription_id, recommendation_id) DO NOTHING`, notificationID, delivery.subscription.ID, userID, event.JourneyID, event.RecommendationID, status)
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
		eventTypes := []string{"notification.sent.v1"}
		if status == "delivered" {
			eventTypes = append(eventTypes, "notification.delivered.v1")
		}
		for _, eventType := range eventTypes {
			if err := enqueueNotificationEvent(ctx, tx, event, eventType, notificationID, channel, providerName, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func enqueueNotificationEvent(ctx context.Context, tx *sql.Tx, source RecommendationCreatedEvent, eventType, notificationID, channel, providerName, failureCode string) error {
	eventID := uuid.NewString()
	eventPayload := map[string]string{
		"notification_id": notificationID,
		"channel":         channel,
		"provider":        providerName,
	}
	if failureCode != "" {
		eventPayload["failure_code"] = failureCode
	}
	payload, err := json.Marshal(map[string]any{
		"event_id":          eventID,
		"event_type":        eventType,
		"schema_version":    1,
		"occurred_at":       time.Now().UTC().Format(time.RFC3339Nano),
		"producer":          "notification-worker",
		"trace_id":          source.TraceID,
		"journey_id":        source.JourneyID,
		"recommendation_id": source.RecommendationID,
		"user_id_hash":      source.UserIDHash,
		"payload":           eventPayload,
	})
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, outbox.Event{ID: eventID, Topic: eventType, Key: source.UserIDHash, Payload: payload})
}
