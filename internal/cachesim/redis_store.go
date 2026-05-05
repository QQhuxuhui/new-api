package cachesim

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

// redisOpTimeout caps each Redis round-trip so a slow or stalled Redis
// instance cannot wedge a Claude request on the cache-simulation hot path.
const redisOpTimeout = 2 * time.Second

// RedisStore implements Store using Redis for checkpoint persistence.
// Each scope is stored as a single JSON value with a TTL that refreshes
// on every access, so inactive scopes are automatically evicted by Redis.
type RedisStore struct {
	client         *redis.Client
	keyPrefix      string
	scopeTTL       time.Duration
	maxCheckpoints int
}

func NewRedisStore(client *redis.Client, maxCheckpoints int) *RedisStore {
	return &RedisStore{
		client:         client,
		keyPrefix:      "cachesim:",
		scopeTTL:       2 * time.Hour,
		maxCheckpoints: maxCheckpoints,
	}
}

func (s *RedisStore) scopeKey(scope ScopeKey) string {
	return fmt.Sprintf("%s%d:%d:%d:%s", s.keyPrefix, scope.UserID, scope.TokenID, scope.ChannelID, scope.Model)
}

func (s *RedisStore) Load(scope ScopeKey) (State, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	key := s.scopeKey(scope)

	data, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return State{}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("redis load: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("redis unmarshal: %w", err)
	}
	return state, nil
}

func (s *RedisStore) Save(scope ScopeKey, state State) error {
	if s.maxCheckpoints > 0 && len(state.Checkpoints) > s.maxCheckpoints {
		state.Checkpoints = append([]Checkpoint(nil), state.Checkpoints[:s.maxCheckpoints]...)
	}

	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("redis marshal: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisOpTimeout)
	defer cancel()
	key := s.scopeKey(scope)
	return s.client.Set(ctx, key, data, s.scopeTTL).Err()
}
