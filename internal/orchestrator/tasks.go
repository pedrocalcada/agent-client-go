package orchestrator

import (
	"agent-client-go/internal/redis"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
)

// NewPlannedTask cria um *a2a.Task que representa uma tarefa a ser enviada a um agente.
// O agente é identificado por Metadata["agent"]; o texto da mensagem fica em History[0].
// Assim a unidade de trabalho é sempre a2a.Task, sem tipo intermediário.
func NewPlannedTask(ctx context.Context, contextID, agentName string) *a2a.Task {
	plannedTask := &a2a.Task{
		ID:        "a2a:task:" + a2a.NewTaskID(),
		ContextID: contextID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateSubmitted},
		Metadata:  map[string]any{"agent": agentName},
	}

	taskData, err := json.Marshal(plannedTask)
	if err != nil {
		log.Printf("erro ao serializar tarefa %s: %v", plannedTask.ID, err)
		return nil
	}
	taskKey := string(plannedTask.ID)
	cmd := redis.Client().Set(ctx, taskKey, taskData, time.Minute*5)
	if err := cmd.Err(); err != nil {
		log.Printf("erro ao persistir tarefa %s: %v", plannedTask.ID, err)
		return nil
	}
	return plannedTask
}

func GetPlannedTask(ctx context.Context, taskID a2a.TaskID) (*a2a.Task, error) {
	cmd := redis.Client().Get(ctx, string(taskID))
	if err := cmd.Err(); err != nil {
		return nil, err
	}
	task := &a2a.Task{}
	if err := json.Unmarshal([]byte(cmd.Val()), task); err != nil {
		return nil, err
	}
	return task, nil
}

func SetPlannedTask(ctx context.Context, task *a2a.Task) error {

	taskData, err := json.Marshal(task)
	if err != nil {
		return err
	}
	taskKey := string(task.ID)
	cmd := redis.Client().Set(ctx, taskKey, taskData, time.Minute*5)
	if err := cmd.Err(); err != nil {
		return err
	}
	return nil
}

// AgentFromTask devolve o nome do agente armazenado em task.Metadata["agent"].
func AgentFromTask(task *a2a.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	a, _ := task.Metadata["agent"].(string)
	return a
}
