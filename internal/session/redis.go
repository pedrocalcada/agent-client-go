package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"agent-client-go/internal/redisclient"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/redis/go-redis/v9"
)

const (
	sessionKeyPrefix = "session:"
	sessionTTL       = 24 * time.Hour
)

// RedisStore persiste o estado das sessões no Redis.
// Usa o client Redis global (redisclient.Client()); não fecha o client.
type RedisStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisStore cria um store que usa o client Redis global (redisclient.Client()).
func NewRedisStore() *RedisStore {
	return &RedisStore{client: redisclient.Client(), ttl: sessionTTL}
}

func sessionKey(clientID string) string {
	return sessionKeyPrefix + clientID
}

func (s *RedisStore) Get(clientID string) (*State, error) {
	ctx := context.Background()
	data, err := s.client.Get(ctx, sessionKey(clientID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("redis session unmarshal: %w", err)
	}
	if state.Tasks == nil {
		state.Tasks = make(map[a2a.TaskID]*a2a.Task)
	}
	return &state, nil
}

func (s *RedisStore) Set(clientID string, state *State) error {
	if state == nil {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("redis session marshal: %w", err)
	}
	ctx := context.Background()
	return s.client.Set(ctx, sessionKey(clientID), data, s.ttl).Err()
}

func (s *RedisStore) GetOrCreate(clientID string) (*State, error) {
	state, err := s.Get(clientID)
	if err != nil {
		return nil, err
	}
	if state != nil {
		return state, nil
	}
	state = &State{
		SessionID: clientID,
		ClientID:  clientID,
		Tasks:     make(map[a2a.TaskID]*a2a.Task),
	}
	if err := s.Set(clientID, state); err != nil {
		return nil, err
	}
	return state, nil
}
