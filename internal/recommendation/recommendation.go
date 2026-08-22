package recommendation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"spacetime-node/internal/platform/observability"
	"spacetime-node/internal/platform/outbox"
)

var (
	ErrInvalidRecommendationRequest = errors.New("invalid recommendation request")
	ErrUserNotFound                 = errors.New("user not found")
	ErrJourneyNotFound              = errors.New("journey not found")
	ErrJourneyStationMismatch       = errors.New("journey station mismatch")
	ErrNoEligibleOffer              = errors.New("no eligible offer")
)

type RecommendationRequest struct {
	UserIDHash string
	JourneyID  string
	StationID  string
	TraceID    string
	Vector     []float32
	Limit      int
}

type CandidateSummary struct {
	OfferID     string   `json:"offer_id"`
	VectorScore float64  `json:"vector_score"`
	RuleScore   float64  `json:"rule_score"`
	Eligible    bool     `json:"eligible"`
	Reasons     []string `json:"reasons"`
}

type Recommendation struct {
	ID                string
	JourneyID         string
	OfferID           string
	Score             float64
	Reasons           []string
	Title             string
	Body              string
	Tone              string
	CopySource        string
	DecisionLatencyMS int64
	Candidates        []CandidateSummary
}

type RecommendationService struct {
	db             *sql.DB
	qdrant         *QdrantClient
	preferences    *PreferenceStore
	collection     string
	queryEmbedder  Embedder
	embeddingModel string
	copyGenerator  CopyGenerator
	copyTimeout    time.Duration
}

func NewRecommendationService(db *sql.DB, qdrant *QdrantClient, preferences *PreferenceStore) *RecommendationService {
	return &RecommendationService{
		db:             db,
		qdrant:         qdrant,
		preferences:    preferences,
		collection:     "offer_embeddings_v1",
		queryEmbedder:  HashEmbedder(DefaultEmbeddingDimension),
		embeddingModel: "demo-hash-v1",
		copyTimeout:    DefaultCopyTimeout,
	}
}

func (s *RecommendationService) WithQueryEmbedder(embedder Embedder) *RecommendationService {
	if s != nil && embedder != nil {
		s.queryEmbedder = embedder
	}
	return s
}

func (s *RecommendationService) WithEmbeddingCollection(collection string) *RecommendationService {
	if s != nil && collection != "" {
		s.collection = collection
	}
	return s
}

func (s *RecommendationService) WithEmbeddingModel(model string) *RecommendationService {
	if s != nil && strings.TrimSpace(model) != "" {
		s.embeddingModel = strings.TrimSpace(model)
	}
	return s
}

func (s *RecommendationService) QueryVector(ctx context.Context, event EntryEvent) ([]float32, error) {
	embedding, err := s.ProfileEmbedding(ctx, event)
	if err != nil {
		return nil, err
	}
	return embedding.Vector, nil
}

// ProfileEmbedding creates a versioned profile vector from stored preferences
// and learned category weights. It is deliberately computed on demand so the
// profile can always be rebuilt without storing a sensitive user vector.
func (s *RecommendationService) ProfileEmbedding(ctx context.Context, event EntryEvent) (ProfileEmbedding, error) {
	if s == nil || s.queryEmbedder == nil || event.UserIDHash == "" || event.Payload.StationID == "" {
		return ProfileEmbedding{}, ErrEmbeddingUnavailable
	}
	preference := DemoPreference(event.UserIDHash)
	if s.preferences != nil {
		var err error
		preference, err = s.preferences.Get(ctx, event.UserIDHash)
		if err != nil {
			return ProfileEmbedding{}, err
		}
	}
	profile := ProfileDocument{
		PredictedDestination: preference.PredictedDestination,
		PreferredCategories:  preference.PreferredCategories,
		CategoryWeights:      preference.CategoryWeights,
		BudgetMinPoints:      preference.BudgetMinPoints,
		BudgetMaxPoints:      preference.BudgetMaxPoints,
	}
	canonical := CanonicalProfileDocument(profile)
	vector, err := s.queryEmbedder(ctx, OfferDocument{
		Title:       "user profile",
		Description: strings.Join([]string{"station=" + event.Payload.StationID, "line=" + event.Payload.LineID, "position=" + event.Payload.PositionID, canonical}, "\n"),
		Category:    "profile",
	})
	if err != nil {
		return ProfileEmbedding{}, err
	}
	return ProfileEmbedding{
		Vector:         vector,
		EmbeddingModel: s.embeddingModel,
		ContentVersion: profileContentVersion(canonical),
	}, nil
}

func (s *RecommendationService) WithCopyGenerator(generator CopyGenerator, timeout time.Duration) *RecommendationService {
	if s == nil {
		return s
	}
	s.copyGenerator = generator
	if timeout > 0 {
		s.copyTimeout = timeout
	}
	return s
}

func (s *RecommendationService) Recommend(ctx context.Context, request RecommendationRequest) (Recommendation, error) {
	started := time.Now()
	if s == nil || s.db == nil || s.qdrant == nil || request.UserIDHash == "" || request.JourneyID == "" || request.StationID == "" || request.TraceID == "" || len(request.Vector) == 0 {
		return Recommendation{}, ErrInvalidRecommendationRequest
	}
	defer observability.RecordDuration(ctx, "recommendation_duration_ms", started, attribute.String("service.name", "recommendation-service"))
	limit := request.Limit
	if limit < 1 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	preference := DemoPreference(request.UserIDHash)
	if s.preferences != nil {
		var err error
		preference, err = s.preferences.Get(ctx, request.UserIDHash)
		if err != nil {
			return Recommendation{}, err
		}
	}
	candidates, err := s.qdrant.Search(ctx, s.collection, request.Vector, request.StationID, limit)
	if err != nil {
		return Recommendation{}, err
	}
	if len(candidates) == 0 {
		return Recommendation{}, ErrNoEligibleOffer
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Recommendation{}, err
	}
	defer tx.Rollback()
	var userID string
	var balance int64
	if err := tx.QueryRowContext(ctx, `SELECT user_id, point_balance FROM users WHERE user_id_hash = $1`, request.UserIDHash).Scan(&userID, &balance); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Recommendation{}, ErrUserNotFound
		}
		return Recommendation{}, err
	}
	var journeyStation, stationName string
	if err := tx.QueryRowContext(ctx, `
		SELECT j.station_id, s.name_zh
		FROM journeys j
		JOIN stations s ON s.station_id = j.station_id
		WHERE j.journey_id = $1 AND j.user_id = $2`, request.JourneyID, userID).Scan(&journeyStation, &stationName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Recommendation{}, ErrJourneyNotFound
		}
		return Recommendation{}, err
	}
	if journeyStation != request.StationID {
		return Recommendation{}, ErrJourneyStationMismatch
	}

	ids := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate.OfferID]; !ok {
			seen[candidate.OfferID] = struct{}{}
			ids = append(ids, candidate.OfferID)
		}
	}
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	args = append(args, journeyStation)
	args = append(args, userID)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args = append(args, id)
	}
	query := fmt.Sprintf(`
		SELECT o.offer_id, o.title, o.description, o.category, o.points_cost, o.station_id,
		       o.is_active, o.starts_at, o.ends_at, i.available_quantity
		FROM offers o
		JOIN inventory i ON i.offer_id = o.offer_id
		WHERE o.station_id = $1
		  AND o.offer_id IN (%s)
		  AND NOT EXISTS (
			SELECT 1 FROM redemptions r
			WHERE r.user_id = $2 AND r.offer_id = o.offer_id
			  AND r.status IN ('succeeded', 'verified')
		  )`, strings.Join(placeholders, ", "))
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return Recommendation{}, err
	}
	type offer struct {
		ID, Title, Description, Category, StationID string
		PointsCost                                  int64
		Active                                      bool
		StartsAt, EndsAt                            time.Time
		AvailableQuantity                           int
	}
	offers := make(map[string]offer, len(ids))
	for rows.Next() {
		var item offer
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.Category, &item.PointsCost, &item.StationID, &item.Active, &item.StartsAt, &item.EndsAt, &item.AvailableQuantity); err != nil {
			rows.Close()
			return Recommendation{}, err
		}
		offers[item.ID] = item
	}
	if err := rows.Close(); err != nil {
		return Recommendation{}, err
	}

	type evaluation struct {
		offer   offer
		summary CandidateSummary
	}
	evaluations := make([]evaluation, 0, len(candidates))
	now := time.Now().UTC()
	for _, candidate := range candidates {
		item, found := offers[candidate.OfferID]
		summary := CandidateSummary{OfferID: candidate.OfferID, VectorScore: candidate.VectorScore, RuleScore: candidate.VectorScore}
		if !found {
			summary.Reasons = []string{"stale_candidate"}
			summary.Eligible = false
			evaluations = append(evaluations, evaluation{summary: summary})
			continue
		}
		if !item.Active || now.Before(item.StartsAt) || !now.Before(item.EndsAt) {
			summary.Reasons = append(summary.Reasons, "offer_inactive_or_expired")
		}
		if item.AvailableQuantity == 0 {
			summary.Reasons = append(summary.Reasons, "out_of_stock")
		}
		if item.PointsCost > balance {
			summary.Reasons = append(summary.Reasons, "insufficient_points")
		}
		if item.StationID == preference.PredictedDestination && preference.PredictedDestination != "" {
			summary.RuleScore += 0.20
			summary.Reasons = append(summary.Reasons, "near_predicted_destination")
		}
		if item.PointsCost >= preference.BudgetMinPoints && item.PointsCost <= preference.BudgetMaxPoints {
			summary.RuleScore += 0.10
			summary.Reasons = append(summary.Reasons, "within_budget")
		}
		if item.AvailableQuantity > 0 && item.AvailableQuantity <= 5 {
			summary.RuleScore += 0.05
			summary.Reasons = append(summary.Reasons, "inventory_urgency")
		}
		if matchesPreferredCategory(item.Category, preference.PreferredCategories) {
			summary.RuleScore += 0.10
			summary.Reasons = append(summary.Reasons, "preferred_category")
		}
		if weight := preference.CategoryWeights[strings.ToLower(strings.TrimSpace(item.Category))]; weight != 0 {
			summary.RuleScore += weight * 0.05
			if weight > 0 {
				summary.Reasons = append(summary.Reasons, "learned_category_preference")
			} else {
				summary.Reasons = append(summary.Reasons, "learned_category_avoidance")
			}
		}
		summary.Eligible = len(summary.Reasons) == 0 || !containsRejection(summary.Reasons)
		if summary.Eligible {
			summary.Reasons = append(summary.Reasons, "within_points")
		}
		evaluations = append(evaluations, evaluation{offer: item, summary: summary})
	}
	sort.SliceStable(evaluations, func(i, j int) bool {
		if evaluations[i].summary.RuleScore == evaluations[j].summary.RuleScore {
			return evaluations[i].summary.OfferID < evaluations[j].summary.OfferID
		}
		return evaluations[i].summary.RuleScore > evaluations[j].summary.RuleScore
	})
	chosenIndex := -1
	for i := range evaluations {
		if evaluations[i].summary.Eligible {
			chosenIndex = i
			break
		}
	}
	if chosenIndex < 0 {
		return Recommendation{}, ErrNoEligibleOffer
	}
	chosen := evaluations[chosenIndex]
	reasonLabels := append([]string{}, chosen.summary.Reasons...)
	if len(reasonLabels) > 3 {
		reasonLabels = reasonLabels[:3]
	}
	copyOutput, copySource, err := GenerateCopy(ctx, CopyFacts{
		StationName:         stationName,
		OfferName:           chosen.offer.Title,
		PointsCost:          chosen.offer.PointsCost,
		DiscountDescription: chosen.offer.Description,
		ReasonLabels:        reasonLabels,
	}, s.copyGenerator, s.copyTimeout)
	if err != nil {
		return Recommendation{}, err
	}
	recommendation := Recommendation{
		ID:                uuid.NewString(),
		JourneyID:         request.JourneyID,
		OfferID:           chosen.offer.ID,
		Score:             chosen.summary.RuleScore,
		Reasons:           chosen.summary.Reasons,
		Title:             copyOutput.Title,
		Body:              copyOutput.Body,
		Tone:              copyOutput.Tone,
		CopySource:        copySource,
		DecisionLatencyMS: time.Since(started).Milliseconds(),
		Candidates:        make([]CandidateSummary, 0, len(evaluations)),
	}
	persistedReasons := append([]string{}, recommendation.Reasons...)
	for _, item := range evaluations {
		recommendation.Candidates = append(recommendation.Candidates, item.summary)
	}
	reasonsJSON, err := json.Marshal(persistedReasons)
	if err != nil {
		return Recommendation{}, err
	}
	candidatesJSON, err := json.Marshal(recommendation.Candidates)
	if err != nil {
		return Recommendation{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recommendations (recommendation_id, journey_id, offer_id, score, reasons, copy_title, copy_body, copy_tone, copy_source, candidate_summary, decision_latency_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`, recommendation.ID, recommendation.JourneyID, recommendation.OfferID, recommendation.Score, reasonsJSON, recommendation.Title, recommendation.Body, recommendation.Tone, recommendation.CopySource, candidatesJSON, recommendation.DecisionLatencyMS); err != nil {
		return Recommendation{}, err
	}
	if err := enqueueRecommendationEvent(ctx, tx, request, recommendation); err != nil {
		return Recommendation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Recommendation{}, err
	}
	return recommendation, nil
}

func matchesPreferredCategory(offerCategory string, preferredCategories []string) bool {
	offerCategory = strings.ToLower(strings.TrimSpace(offerCategory))
	if offerCategory == "" {
		return false
	}
	for _, preferred := range preferredCategories {
		if offerCategory == strings.ToLower(strings.TrimSpace(preferred)) {
			return true
		}
	}
	return false
}

func (s *RecommendationService) GetLatest(ctx context.Context, journeyID string) (Recommendation, error) {
	if s == nil || s.db == nil || journeyID == "" {
		return Recommendation{}, ErrInvalidRecommendationRequest
	}
	var recommendation Recommendation
	var reasonsJSON []byte
	var candidatesJSON []byte
	var title, body, tone, source sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT recommendation_id, journey_id, offer_id, score, reasons,
		       copy_title, copy_body, copy_tone, copy_source, candidate_summary, decision_latency_ms
		FROM recommendations
		WHERE journey_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, journeyID).Scan(&recommendation.ID, &recommendation.JourneyID, &recommendation.OfferID, &recommendation.Score, &reasonsJSON, &title, &body, &tone, &source, &candidatesJSON, &recommendation.DecisionLatencyMS)
	if errors.Is(err, sql.ErrNoRows) {
		return Recommendation{}, ErrNoEligibleOffer
	}
	if err != nil {
		return Recommendation{}, err
	}
	if err := json.Unmarshal(reasonsJSON, &recommendation.Reasons); err != nil {
		return Recommendation{}, err
	}
	if err := json.Unmarshal(candidatesJSON, &recommendation.Candidates); err != nil {
		return Recommendation{}, err
	}
	recommendation.Title = title.String
	recommendation.Body = body.String
	recommendation.Tone = tone.String
	recommendation.CopySource = source.String
	return recommendation, nil
}

func containsRejection(reasons []string) bool {
	for _, reason := range reasons {
		if strings.HasPrefix(reason, "offer_inactive") || reason == "out_of_stock" || reason == "insufficient_points" {
			return true
		}
	}
	return false
}

func enqueueRecommendationEvent(ctx context.Context, tx *sql.Tx, request RecommendationRequest, recommendation Recommendation) error {
	eventID := uuid.NewString()
	message, err := json.Marshal(struct {
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
	}{
		EventID:          eventID,
		EventType:        "recommendation.created.v1",
		SchemaVersion:    1,
		OccurredAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Producer:         "recommendation-service",
		TraceID:          request.TraceID,
		JourneyID:        request.JourneyID,
		RecommendationID: recommendation.ID,
		UserIDHash:       request.UserIDHash,
		Payload: struct {
			RecommendationID string   `json:"recommendation_id"`
			OfferID          string   `json:"offer_id"`
			Reasons          []string `json:"reasons"`
			CopySource       string   `json:"copy_source"`
		}{
			RecommendationID: recommendation.ID,
			OfferID:          recommendation.OfferID,
			Reasons:          recommendation.Reasons,
			CopySource:       recommendation.CopySource,
		},
	})
	if err != nil {
		return err
	}
	return outbox.Enqueue(ctx, tx, outbox.Event{
		ID:      eventID,
		Topic:   "recommendation.created.v1",
		Key:     request.UserIDHash,
		Payload: message,
	})
}
