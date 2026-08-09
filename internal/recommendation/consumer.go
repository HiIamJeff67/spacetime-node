package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/segmentio/kafka-go"
)

const (
	EntryTopic                    = "journey.entered.v1"
	EntryConsumerGroup            = "recommendation-service-v1"
	RecommendationVectorDimension = 32
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
	embed := HashEmbedder(RecommendationVectorDimension)
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		event, err := DecodeEntryEvent(message.Value)
		if err != nil {
			logger.Printf("skip invalid %s message: %v", EntryTopic, err)
			if err := reader.CommitMessages(ctx, message); err != nil {
				return err
			}
			continue
		}

		// A committed recommendation makes a replay safe for the demo. The
		// recommendation transaction itself remains the source of truth.
		if _, err := service.GetLatest(ctx, event.JourneyID); err == nil {
			if err := reader.CommitMessages(ctx, message); err != nil {
				return err
			}
			continue
		}
		vector, err := embed(ctx, OfferDocument{
			Title:       event.Payload.StationID,
			Description: strings.TrimSpace(event.Payload.LineID + " " + event.Payload.PositionID),
		})
		if err != nil {
			return err
		}
		_, err = service.Recommend(ctx, RecommendationRequest{
			UserIDHash: event.UserIDHash,
			JourneyID:  event.JourneyID,
			StationID:  event.Payload.StationID,
			TraceID:    event.TraceID,
			Vector:     vector,
			Limit:      10,
		})
		if err != nil {
			logger.Printf("recommendation failed for journey %s: %v", event.JourneyID, err)
			continue
		}
		if err := reader.CommitMessages(ctx, message); err != nil {
			return err
		}
	}
}
