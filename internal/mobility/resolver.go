package mobility

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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
	BID               string `json:"BID"`
	SID               string `json:"SID"`
	LID               string `json:"LID"`
	POSINO            string `json:"POSINO"`
	POSITION          string `json:"POSITION"`
	STATIONID         string `json:"STATIONID"`
	StationName       string `json:"STATION_NAME"`
	StationNameLegacy string `json:"STATIONNAME"`
}

type Context struct {
	StationID   string  `json:"station_id"`
	LineID      string  `json:"line_id"`
	PositionID  string  `json:"position_id"`
	StationName string  `json:"station_name"`
	Source      string  `json:"source"`
	Confidence  float64 `json:"confidence"`
	NearExit    bool    `json:"near_exit"`
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
	redis    *redis.Client
	mu       sync.Mutex
	cache    map[string]Context
}

const beaconContextCacheTTL = 15 * time.Minute

type Provider interface {
	Resolve(context.Context, Observation) (ProviderResponse, error)
}

func NewResolver(provider Provider, mappings map[string]StationMapping, fixtures map[string]Context, fallback Context, redisClients ...*redis.Client) *Resolver {
	var redisClient *redis.Client
	if len(redisClients) > 0 {
		redisClient = redisClients[0]
	}
	return &Resolver{provider: provider, mappings: mappings, fixtures: fixtures, fallback: fallback, redis: redisClient, cache: make(map[string]Context)}
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
	if cached, ok := r.loadRedis(ctx, key); ok {
		r.storeLocal(key, cached)
		cached.Source = "cache"
		return cached, nil
	}

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
	sid := strings.ToUpper(strings.TrimSpace(response.SID))
	lid := strings.ToUpper(strings.TrimSpace(response.LID))
	key := sid + "|" + lid
	mapping, ok := r.mappings[key]
	if !ok {
		stationName := strings.TrimSpace(response.StationName)
		if stationName == "" {
			stationName = strings.TrimSpace(response.StationNameLegacy)
		}
		if sid == "" || lid == "" || stationName == "" {
			return Context{}, ErrUnmappedBeacon
		}
		mapping = StationMapping{StationID: sid, LineID: lid, StationName: stationName}
	}
	if mapping.StationID == "" {
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
	r.storeLocal(key, value)
	if r.redis == nil {
		return
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	// Cache failures are intentionally best-effort; fixture fallback keeps the
	// Beacon entry path available when Redis is temporarily unavailable.
	_ = r.redis.Set(context.Background(), redisCacheKey(key), encoded, beaconContextCacheTTL).Err()
}

func (r *Resolver) storeLocal(key string, value Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = value
}

func (r *Resolver) loadRedis(ctx context.Context, key string) (Context, bool) {
	if r.redis == nil {
		return Context{}, false
	}
	encoded, err := r.redis.Get(ctx, redisCacheKey(key)).Bytes()
	if err != nil {
		return Context{}, false
	}
	var value Context
	if json.Unmarshal(encoded, &value) != nil || value.StationID == "" {
		return Context{}, false
	}
	return value, true
}

func redisCacheKey(key string) string { return "mobility:beacon:" + key }

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
