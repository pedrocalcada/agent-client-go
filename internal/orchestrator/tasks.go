package orchestrator

import (
	"github.com/a2aproject/a2a-go/a2a"
)

// NewPlannedTask cria um *a2a.Task que representa uma tarefa a ser enviada a um agente.
// O agente é identificado por Metadata["agent"]; o texto da mensagem fica em History[0].
// Assim a unidade de trabalho é sempre a2a.Task, sem tipo intermediário.
func NewPlannedTask(contextID, agentName, messageText string) *a2a.Task {
	return &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: contextID,
		Status:    a2a.TaskStatus{State: a2a.TaskStateSubmitted},
		History: []*a2a.Message{{
			ID:    a2a.NewMessageID(),
			Role:  a2a.MessageRoleUser,
			Parts: a2a.ContentParts{a2a.TextPart{Text: messageText}},
		}},
		Metadata: map[string]any{"agent": agentName},
	}
}

// AgentFromTask devolve o nome do agente armazenado em task.Metadata["agent"].
func AgentFromTask(task *a2a.Task) string {
	if task == nil || task.Metadata == nil {
		return ""
	}
	a, _ := task.Metadata["agent"].(string)
	return a
}

// MessageTextFromPlannedTask devolve o texto da última mensagem (user) do History, para envio ao agente.
// Assim funciona tanto na primeira execução (uma mensagem) quanto na continuação após InputRequired (várias mensagens).
func MessageTextFromPlannedTask(task *a2a.Task) string {
	if task == nil || len(task.History) == 0 {
		return ""
	}
	for i := len(task.History) - 1; i >= 0; i-- {
		if task.History[i].Role == a2a.MessageRoleUser && len(task.History[i].Parts) > 0 {
			return textFromParts(task.History[i].Parts)
		}
	}
	return ""
}

// UpdateTaskFromWorking atualiza a task com o estado retornado pelo agente (TaskStateWorking).
// Assim a task em state.Tasks reflete o estado do agente e pode ser usada para continuar a conversa.
func UpdateTaskFromWorking(planned, working *a2a.Task) {
	if planned == nil || working == nil {
		return
	}

	planned.Status = working.Status
	planned.ID = working.ID
	if len(working.History) > 0 {
		planned.History = append(planned.History, working.History...)
	}
	if len(working.Artifacts) > 0 {
		if planned.Artifacts == nil {
			planned.Artifacts = make([]*a2a.Artifact, 0, len(working.Artifacts))
		}
		planned.Artifacts = append(planned.Artifacts, working.Artifacts...)
	}
}
