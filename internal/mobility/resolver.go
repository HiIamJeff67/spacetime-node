package mobility

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var (
	ErrInvalidObservation = errors.New("invalid beacon observation")
	ErrBeaconUnavailable  = errors.New("beacon context unavailable")
	ErrUnmappedBeacon     = errors.New("beacon station is unmapped")
)

type Observation struct {
	UUID  string
	Major int64
	Minor int64
	Power int32
}

type ProviderResponse struct {
	BID         string `json:"BID"`
	SID         string `json:"SID"`
	LID         string `json:"LID"`
	POSINO      string `json:"POSINO"`
	POSITION    string `json:"POSITION"`
	STATIONID   string `json:"STATIONID"`
	StationName string `json:"STATION_NAME"`
}

type Context struct {
	StationID   string
	LineID      string
	PositionID  string
	StationName string
	Source      string
	Confidence  float64
	NearExit    bool
}

type StationMapping struct {
	StationID   string
	LineID      string
	StationName string
}

type Resolver struct {
	provider Provider
	mappings map[string]StationMapping
	fixtures map[string]Context
	fallback Context
	mu       sync.Mutex
	cache    map[string]Context
}

type Provider interface {
	Resolve(context.Context, Observation) (ProviderResponse, error)
}

func NewResolver(provider Provider, mappings map[string]StationMapping, fixtures map[string]Context, fallback Context) *Resolver {
	return &Resolver{provider: provider, mappings: mappings, fixtures: fixtures, fallback: fallback, cache: make(map[string]Context)}
}

func (r *Resolver) Resolve(ctx context.Context, observation Observation) (Context, error) {
	if r == nil || strings.TrimSpace(observation.UUID) == "" || observation.Major < 0 || observation.Minor < 0 {
		return Context{}, ErrInvalidObservation
	}
	key := observationKey(observation)
	r.mu.Lock()
	if cached, ok := r.cache[key]; ok {
		cached.Source = "cache"
		r.mu.Unlock()
		return cached, nil
	}
	r.mu.Unlock()

	if r.provider != nil {
		if response, err := r.provider.Resolve(ctx, observation); err == nil {
			if resolved, err := r.normalize(response, "provider"); err == nil {
				r.store(key, resolved)
				return resolved, nil
			}
		}
	}
	if fixture, ok := r.fixtures[key]; ok {
		fixture.Source = "fixture"
		if fixture.Confidence == 0 {
			fixture.Confidence = 0.75
		}
		return fixture, nil
	}
	if r.fallback.StationID != "" {
		fallback := r.fallback
		fallback.Source = "station_fixture"
		if fallback.Confidence == 0 {
			fallback.Confidence = 0.5
		}
		return fallback, nil
	}
	return Context{}, ErrBeaconUnavailable
}

func (r *Resolver) normalize(response ProviderResponse, source string) (Context, error) {
	key := strings.ToUpper(strings.TrimSpace(response.SID)) + "|" + strings.TrimSpace(response.LID)
	mapping, ok := r.mappings[key]
	if !ok || mapping.StationID == "" {
		return Context{}, ErrUnmappedBeacon
	}
	position := strings.TrimSpace(response.POSINO)
	if position == "" {
		position = strings.TrimSpace(response.POSITION)
	}
	nearExit := strings.Contains(strings.ToLower(position), "exit") || strings.Contains(position, "出口") || strings.Contains(position, "出站")
	return Context{StationID: mapping.StationID, LineID: mapping.LineID, PositionID: position, StationName: mapping.StationName, Source: source, Confidence: 0.95, NearExit: nearExit}, nil
}

func (r *Resolver) store(key string, value Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = value
}

func observationKey(observation Observation) string {
	return strings.ToLower(strings.TrimSpace(observation.UUID)) + ":" + formatInt(observation.Major) + ":" + formatInt(observation.Minor)
}

func formatInt(value int64) string {
	// ponytail: avoid fmt allocation for the tiny cache key while keeping the resolver dependency-free.
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}
