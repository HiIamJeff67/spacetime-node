package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/segmentio/kafka-go"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPublisherRetriesPendingEvent(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	insertEvent(t, db, "event-retry")

	writer := &recordingWriter{err: errors.New("kafka unavailable")}
	publisher := NewPublisher(db, writer)
	if _, err := publisher.PublishPending(context.Background(), 10); err == nil {
		t.Fatal("expected publish failure")
	}

	var attempts int
	var publishedAt sql.NullTime
	if err := db.QueryRow(`SELECT publish_attempts, published_at FROM outbox_events WHERE event_id = 'event-retry'`).Scan(&attempts, &publishedAt); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || publishedAt.Valid {
		t.Fatalf("failed event was not retained: attempts=%d published=%t", attempts, publishedAt.Valid)
	}

	writer.err = nil
	published, err := publisher.PublishPending(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if published != 1 || len(writer.messages) != 1 || writer.messages[0].Topic != "redemption.succeeded.v1" {
		t.Fatalf("unexpected publish result: published=%d messages=%d", published, len(writer.messages))
	}
	if err := db.QueryRow(`SELECT published_at FROM outbox_events WHERE event_id = 'event-retry'`).Scan(&publishedAt); err != nil {
		t.Fatal(err)
	}
	if !publishedAt.Valid {
		t.Fatal("successfully sent event was not marked published")
	}
}

func TestProcessDeduplicatesSideEffect(t *testing.T) {
	db := integrationDB(t)
	resetDatabase(t, db)
	handler := func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO outbox_events (event_id, topic, event_key, payload)
			VALUES ('event-side-effect', 'dlq.v1', 'key', '{"event_id":"event-side-effect"}')`)
		return err
	}

	processed, err := Process(context.Background(), db, "analytics-consumer", "event-duplicate", handler)
	if err != nil || !processed {
		t.Fatalf("first process result: processed=%t err=%v", processed, err)
	}
	processed, err = Process(context.Background(), db, "analytics-consumer", "event-duplicate", handler)
	if err != nil || processed {
		t.Fatalf("replayed process result: processed=%t err=%v", processed, err)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM outbox_events WHERE event_id = 'event-side-effect'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate event repeated side effect %d times", count)
	}
}

type recordingWriter struct {
	err      error
	messages []kafka.Message
}

func (w *recordingWriter) WriteMessages(_ context.Context, messages ...kafka.Message) error {
	if w.err != nil {
		return w.err
	}
	w.messages = append(w.messages, messages...)
	return nil
}

func insertEvent(t *testing.T, db *sql.DB, eventID string) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"event_id": eventID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO outbox_events (event_id, topic, event_key, payload)
		VALUES ($1, 'redemption.succeeded.v1', 'sha256:test', $2)`, eventID, payload); err != nil {
		t.Fatal(err)
	}
}

func integrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run PostgreSQL integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func resetDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "postgres", "000001_core.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(contents)); err != nil {
		t.Fatal(err)
	}
}
