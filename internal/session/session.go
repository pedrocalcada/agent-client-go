package session

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"agent-client-go/internal/planner"
	"agent-client-go/internal/redis"

	"github.com/a2aproject/a2a-go/a2a"
	goredis "github.com/redis/go-redis/v9"
)

const (
	sessionKeyPrefix = "session:"
	sessionTTL       = 10 * time.Minute
)

// TaskItem representa uma tarefa do planner associada à sessão (TaskID preenchido quando o agente retorna Working).
type TaskItem struct {
	Order   int        `json:"order"`
	Agent   string     `json:"agent"`
	Message string     `json:"message"`
	TaskID  a2a.TaskID `json:"task_id"`
}

// State guarda o estado da conversa de um cliente (identificado por id_cliente).
type State struct {
	ClientID                string                          // id_cliente
	InConversationWithAgent a2a.TaskID                      // se não for zero, é o ID da task aguardando resposta do usuário (InputRequired)
	TaskItems               map[a2a.TaskID]planner.TaskItem // tarefas do planner (TaskID preenchido quando o agente retorna Working)
	CompletedTasks          map[a2a.TaskID]planner.TaskItem // tarefas completadas
	OriginalMsg             string                          // mensagem original da primeira interação
	CurrentMsg              string                          // mensagem atual (primeira interação = OriginalMsg; continuação = resposta do usuário)
}

// SessionKey retorna a chave Redis para o clientID.
func SessionKey(clientID string) string {
	return sessionKeyPrefix + clientID
}

// GetSession carrega o estado da sessão do cliente no Redis.
func GetSession(ctx context.Context, clientID string) (*State, error) {
	cmd := redis.Client().Get(ctx, SessionKey(clientID))
	if err := cmd.Err(); err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal([]byte(cmd.Val()), &state); err != nil {
		return nil, fmt.Errorf("session unmarshal: %w", err)
	}
	return &state, nil
}

// SetSession persiste o estado da sessão no Redis (com TTL).
func SetSession(ctx context.Context, clientID string, state *State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("session marshal: %w", err)
	}
	return redis.Client().Set(ctx, SessionKey(clientID), data, sessionTTL).Err()
}

// GetOrCreateSession retorna a sessão existente ou cria uma nova e persiste no Redis.
func GetOrCreateSession(ctx context.Context, clientID string) (*State, error) {
	state, err := GetSession(ctx, clientID)
	if err != nil {
		if err == goredis.Nil {
			state = &State{ClientID: clientID}
			if err := SetSession(ctx, clientID, state); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return state, nil
}
