package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

var ErrInvalidInput = errors.New("invalid outbox input")

type Event struct {
	ID      string
	Topic   string
	Key     string
	Payload json.RawMessage
}

func Enqueue(ctx context.Context, tx *sql.Tx, event Event) error {
	if tx == nil || event.ID == "" || event.Topic == "" || event.Key == "" || len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return ErrInvalidInput
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_events (event_id, topic, event_key, payload)
		VALUES ($1, $2, $3, $4)`, event.ID, event.Topic, event.Key, event.Payload)
	return err
}

type Writer interface {
	WriteMessages(context.Context, ...kafka.Message) error
}

type Publisher struct {
	db     *sql.DB
	writer Writer
}

func NewPublisher(db *sql.DB, writer Writer) *Publisher {
	return &Publisher{db: db, writer: writer}
}

func NewKafkaPublisher(db *sql.DB, brokers []string) *Publisher {
	return NewPublisher(db, &kafka.Writer{Addr: kafka.TCP(brokers...)})
}

func RunPublisher(ctx context.Context, publisher *Publisher, limit int, interval time.Duration, logger *log.Logger) {
	if publisher == nil || limit < 1 {
		return
	}
	if interval <= 0 {
		interval = time.Second
	}
	if logger == nil {
		logger = log.Default()
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := publisher.PublishPending(ctx, limit); err != nil && ctx.Err() == nil {
			logger.Printf("outbox publish failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *Publisher) PublishPending(ctx context.Context, limit int) (int, error) {
	if p == nil || p.db == nil || p.writer == nil || limit < 1 {
		return 0, ErrInvalidInput
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT event_id, topic, event_key, payload
		FROM outbox_events
		WHERE published_at IS NULL
		ORDER BY occurred_at, event_id
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return 0, err
	}

	var events []Event
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Topic, &event.Key, &event.Payload); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, event)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	published := 0
	for _, event := range events {
		err := p.writer.WriteMessages(ctx, kafka.Message{
			Topic: event.Topic,
			Key:   []byte(event.Key),
			Value: event.Payload,
		})
		if err != nil {
			if _, updateErr := tx.ExecContext(ctx, `
				UPDATE outbox_events
				SET publish_attempts = publish_attempts + 1, last_error = $1
				WHERE event_id = $2`, err.Error(), event.ID); updateErr != nil {
				return published, updateErr
			}
			if commitErr := tx.Commit(); commitErr != nil {
				return published, commitErr
			}
			return published, fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE outbox_events
			SET published_at = current_timestamp, last_error = NULL
			WHERE event_id = $1`, event.ID); err != nil {
			return published, err
		}
		published++
	}
	if err := tx.Commit(); err != nil {
		return published, err
	}
	return published, nil
}

func Process(ctx context.Context, db *sql.DB, consumerName, eventID string, handler func(context.Context, *sql.Tx) error) (bool, error) {
	if db == nil || consumerName == "" || eventID == "" {
		return false, ErrInvalidInput
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		INSERT INTO processed_events (consumer_name, event_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, consumerName, eventID)
	if err != nil {
		return false, err
	}
	processed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if processed == 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	if handler != nil {
		if err := handler(ctx, tx); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
