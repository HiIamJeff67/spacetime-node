package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/attribute"
	"spacetime-node/internal/platform/observability"
)

const (
	EntryTopic                    = "journey.entered.v1"
	EntryConsumerGroup            = "recommendation-service-v1"
	RecommendationVectorDimension = DefaultEmbeddingDimension
)

type EntryEvent struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	SchemaVersion int    `json:"schema_version"`
	TraceID       string `json:"trace_id"`
	JourneyID     string `json:"journey_id"`
	UserIDHash    string `json:"user_id_hash"`
	Payload       struct {
		StationID  string `json:"station_id"`
		LineID     string `json:"line_id"`
		PositionID string `json:"position_id"`
	} `json:"payload"`
}

func DecodeEntryEvent(data []byte) (EntryEvent, error) {
	var event EntryEvent
	if json.Unmarshal(data, &event) != nil || event.EventID == "" || event.EventType != EntryTopic || event.SchemaVersion != 1 || event.TraceID == "" || event.JourneyID == "" || event.UserIDHash == "" || event.Payload.StationID == "" {
		return EntryEvent{}, errors.New("invalid journey entered event")
	}
	return event, nil
}

func RunEntryConsumer(ctx context.Context, brokers []string, service *RecommendationService, logger *log.Logger) error {
	if len(brokers) == 0 || service == nil {
		return errors.New("recommendation consumer dependencies are missing")
	}
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokers,
		Topic:    EntryTopic,
		GroupID:  EntryConsumerGroup,
		MinBytes: 1,
		MaxBytes: 10e6,
	})
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
		eventCtx, span := observability.StartKafkaSpan(eventCtx, EntryTopic, "", true)
		event, err := DecodeEntryEvent(message.Value)
		if err != nil {
			observability.Logf(eventCtx, logger, "skip invalid message", attribute.String("event_id", string(message.Key)), attribute.String("topic", EntryTopic))
			if err := reader.CommitMessages(eventCtx, message); err != nil {
				span.RecordError(err)
				span.End()
				return err
			}
			span.End()
			continue
		}

		// A committed recommendation makes a replay safe for the demo. The
		// recommendation transaction itself remains the source of truth.
		if _, err := service.GetLatest(ctx, event.JourneyID); err == nil {
			if err := reader.CommitMessages(eventCtx, message); err != nil {
				span.RecordError(err)
				span.End()
				return err
			}
			span.End()
			continue
		}
		vector, err := service.QueryVector(eventCtx, event)
		if err != nil {
			span.RecordError(err)
			span.End()
			return err
		}
		_, err = service.Recommend(eventCtx, RecommendationRequest{
			UserIDHash: event.UserIDHash,
			JourneyID:  event.JourneyID,
			StationID:  event.Payload.StationID,
			TraceID:    event.TraceID,
			Vector:     vector,
			Limit:      service.CandidateLimit(),
		})
		if err != nil {
			observability.Logf(eventCtx, logger, "recommendation failed", attribute.String("event_id", event.EventID), attribute.String("journey_id", event.JourneyID), attribute.String("error", err.Error()))
			span.RecordError(err)
			span.End()
			continue
		}
		span.SetAttributes(attribute.String("messaging.message.id", event.EventID), attribute.String("journey.id", event.JourneyID))
		if err := reader.CommitMessages(eventCtx, message); err != nil {
			span.RecordError(err)
			span.End()
			return err
		}
		observability.RecordValue(eventCtx, "kafka_consumer_lag", int64(reader.Stats().Lag), attribute.String("messaging.destination.name", EntryTopic))
		span.End()
	}
}
