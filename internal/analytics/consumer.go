package analytics

import (
	"context"
	"errors"
	"log"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel/attribute"
	"spacetime-node/internal/platform/observability"
)

func Run(ctx context.Context, brokers []string, writer Writer, logger *log.Logger) error {
	if len(brokers) == 0 || writer == nil {
		return errors.New("analytics consumer dependencies are missing")
	}
	if logger == nil {
		logger = log.Default()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errors := make(chan error, len(Topics))
	for _, topic := range Topics {
		go func(topic string) { errors <- consumeTopic(ctx, brokers, topic, writer, logger) }(topic)
	}
	for range Topics {
		err := <-errors
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func consumeTopic(ctx context.Context, brokers []string, topic string, writer Writer, logger *log.Logger) error {
	reader := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: topic, GroupID: ConsumerGroup, MinBytes: 1, MaxBytes: 10e6})
	defer reader.Close()
	for {
		message, err := reader.FetchMessage(ctx)
		if err != nil {
			return err
		}
		eventCtx := observability.ExtractKafka(ctx, message.Headers)
		eventCtx, span := observability.StartKafkaSpan(eventCtx, topic, "", true)
		event, err := DecodeEvent(message.Value)
		if err != nil {
			observability.Logf(eventCtx, logger, "skip invalid event", attribute.String("event_id", string(message.Key)), attribute.String("topic", topic))
		} else if err = writer.Insert(eventCtx, event); err != nil {
			span.RecordError(err)
			span.End()
			return err
		} else {
			span.SetAttributes(attribute.String("messaging.message.id", event.EventID), attribute.String("journey.id", event.JourneyID))
		}
		if err := reader.CommitMessages(eventCtx, message); err != nil {
			span.RecordError(err)
			span.End()
			return err
		}
		observability.RecordValue(eventCtx, "kafka_consumer_lag", int64(reader.Stats().Lag), attribute.String("messaging.destination.name", topic))
		span.End()
	}
}
