package mobility

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestResolverSharesProviderContextThroughRedis(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("set TEST_REDIS_ADDR to run Redis cache integration test")
	}
	client := redis.NewClient(&redis.Options{Addr: address})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis is unavailable: %v", err)
	}

	observation := Observation{UUID: uuid.NewString(), Major: 1, Minor: 1}
	key := observationKey(observation)
	t.Cleanup(func() { _ = client.Del(context.Background(), redisCacheKey(key)).Err() })

	providerContext := ProviderResponse{SID: "R04", LID: "R", POSINO: "exit-3", StationName: "信義安和"}
	first := NewResolver(fakeProvider{response: providerContext}, map[string]StationMapping{
		"R04|R": {StationID: "R04", LineID: "R", StationName: "信義安和"},
	}, nil, Context{}, client)
	if resolved, err := first.Resolve(context.Background(), observation); err != nil || resolved.Source != "provider" {
		t.Fatalf("expected provider resolution, got %+v, %v", resolved, err)
	}

	second := NewResolver(fakeProvider{err: errors.New("provider unavailable")}, nil, nil, Context{}, client)
	resolved, err := second.Resolve(context.Background(), observation)
	if err != nil || resolved.StationID != "R04" || resolved.Source != "cache" {
		t.Fatalf("expected Redis cached context, got %+v, %v", resolved, err)
	}
}
