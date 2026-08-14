package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/segmentio/kafka-go"

	"spacetime-node/internal/analytics"
	"spacetime-node/internal/platform/config"
)

func main() {
	journeyID := flag.String("journey-id", "demo-journey-1", "journey to attribute engagement to")
	recommendationID := flag.String("recommendation-id", "demo-recommendation-1", "recommendation to attribute engagement to")
	userIDHash := flag.String("user-id-hash", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "hashed demo user identifier")
	traceID := flag.String("trace-id", "demo-trace-1", "trace identifier")
	flag.Parse()

	dependencies := config.LoadDependencies()
	if dependencies.KafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is required")
	}
	events, err := analytics.DemoEngagementEvents(*journeyID, *recommendationID, *userIDHash, *traceID)
	if err != nil {
		log.Fatal(err)
	}
	writer := &kafka.Writer{Addr: kafka.TCP(strings.Split(dependencies.KafkaBrokers, ",")...), RequiredAcks: kafka.RequireOne}
	defer writer.Close()
	for _, event := range events {
		decoded, err := analytics.DecodeEvent(event)
		if err != nil {
			log.Fatal(err)
		}
		if err := writer.WriteMessages(context.Background(), kafka.Message{Topic: decoded.EventType, Key: []byte(*userIDHash), Value: event}); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("published %d engagement events", len(events))
}
