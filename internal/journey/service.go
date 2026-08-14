package journey

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	v1 "spacetime-node/api/proto/spacetime_node/v1"
	"spacetime-node/internal/platform/outbox"
)

const entryTopic = "journey.entered.v1"

type Service struct {
	v1.UnimplementedJourneyServiceServer
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CreateEntryEvent(ctx context.Context, request *v1.CreateEntryEventRequest) (*v1.CreateEntryEventResponse, error) {
	if s == nil || s.db == nil || request == nil || request.GetUserIdHash() == "" || request.GetStationId() == "" {
		return nil, v1.ErrorInvalidRequest("user_id_hash and station_id are required")
	}
	if !validUserIDHash(request.GetUserIdHash()) {
		return nil, v1.ErrorInvalidRequest("user_id_hash must be a sha256 hash")
	}

	traceID := ""
	if request.GetRequestContext() != nil {
		traceID = request.GetRequestContext().GetTraceId()
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	journeyID := uuid.NewString()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var userID string
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM users WHERE user_id_hash = $1`, request.GetUserIdHash()).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, v1.ErrorJourneyNotFound("user is not registered")
		}
		return nil, err
	}
	var stationLineID string
	if err := tx.QueryRowContext(ctx, `SELECT line_id FROM stations WHERE station_id = $1`, request.GetStationId()).Scan(&stationLineID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, v1.ErrorInvalidRequest("station is not registered")
		}
		return nil, err
	}
	lineID := request.GetLineId()
	if lineID == "" {
		lineID = stationLineID
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO journeys (journey_id, user_id, station_id, line_id, position_id)
		VALUES ($1, $2, $3, $4, $5)`, journeyID, userID, request.GetStationId(), lineID, request.GetPositionId()); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(struct {
		StationID  string `json:"station_id"`
		LineID     string `json:"line_id,omitempty"`
		PositionID string `json:"position_id,omitempty"`
	}{
		StationID:  request.GetStationId(),
		LineID:     lineID,
		PositionID: request.GetPositionId(),
	})
	if err != nil {
		return nil, err
	}
	eventID := uuid.NewString()
	event, err := json.Marshal(struct {
		EventID       string          `json:"event_id"`
		EventType     string          `json:"event_type"`
		SchemaVersion int             `json:"schema_version"`
		OccurredAt    string          `json:"occurred_at"`
		Producer      string          `json:"producer"`
		TraceID       string          `json:"trace_id"`
		JourneyID     string          `json:"journey_id"`
		UserIDHash    string          `json:"user_id_hash"`
		Payload       json.RawMessage `json:"payload"`
	}{
		EventID:       eventID,
		EventType:     entryTopic,
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Producer:      "gateway-service",
		TraceID:       traceID,
		JourneyID:     journeyID,
		UserIDHash:    request.GetUserIdHash(),
		Payload:       payload,
	})
	if err != nil {
		return nil, err
	}
	if err := outbox.Enqueue(ctx, tx, outbox.Event{
		ID:      eventID,
		Topic:   entryTopic,
		Key:     request.GetUserIdHash(),
		Payload: event,
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &v1.CreateEntryEventResponse{JourneyId: journeyID}, nil
}

func (s *Service) GetLatestRecommendation(ctx context.Context, request *v1.GetLatestRecommendationRequest) (*v1.GetLatestRecommendationResponse, error) {
	if s == nil || s.db == nil || request == nil || request.GetJourneyId() == "" {
		return nil, v1.ErrorInvalidRequest("journey_id is required")
	}
	var response v1.GetLatestRecommendationResponse
	var reasonsJSON []byte
	var candidatesJSON []byte
	var title, body, source sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT recommendation_id, journey_id, offer_id, copy_title, copy_body, reasons, copy_source, candidate_summary, decision_latency_ms
		FROM recommendations
		WHERE journey_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, request.GetJourneyId()).Scan(
		&response.RecommendationId, &response.JourneyId, &response.OfferId,
		&title, &body, &reasonsJSON, &source, &candidatesJSON, &response.DecisionLatencyMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, v1.ErrorRecommendationNotFound("recommendation is not ready")
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(reasonsJSON, &response.Reasons); err != nil {
		return nil, err
	}
	var candidates []struct {
		OfferID     string   `json:"offer_id"`
		VectorScore float64  `json:"vector_score"`
		RuleScore   float64  `json:"rule_score"`
		Eligible    bool     `json:"eligible"`
		Reasons     []string `json:"reasons"`
	}
	if err := json.Unmarshal(candidatesJSON, &candidates); err != nil {
		return nil, err
	}
	response.Candidates = make([]*v1.RecommendationCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		response.Candidates = append(response.Candidates, &v1.RecommendationCandidate{
			OfferId: candidate.OfferID, VectorScore: candidate.VectorScore, RuleScore: candidate.RuleScore,
			Eligible: candidate.Eligible, Reasons: candidate.Reasons,
		})
	}
	response.Title = title.String
	response.Body = body.String
	response.CopySource = source.String
	return &response, nil
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
