package session

import (
	"encoding/json"
	"strings"

	"agent-client-go/internal/planner"

	"github.com/a2aproject/a2a-go/a2a"
)

// ParseIncomingMessage interpreta o body do SQS. Se for JSON com id_cliente e mensagem/texto, preenche;
// senão trata o body inteiro como texto e ClientID fica vazio.
func ParseIncomingMessage(body string) (clientID string, text string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
	}
	var m struct {
		ClientID string `json:"id_cliente"`
		Text     string `json:"mensagem"`
		TextAlt  string `json:"texto"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", body
	}
	msg := m.Text
	if msg == "" {
		msg = m.TextAlt
	}
	return m.ClientID, msg
}

// State guarda o estado da conversa de um cliente (identificado por id_cliente).
type State struct {
	SessionID               string                   // id de sessão (criado na primeira mensagem)
	ClientID                string                   // id_cliente
	InConversationWithAgent a2a.TaskID              // se não for zero, é o ID da task aguardando resposta do usuário (InputRequired)
	TaskItems               []planner.TaskItem       // tarefas do planner (TaskID preenchido quando o agente retorna Working)
	Tasks                   map[a2a.TaskID]*a2a.Task // key = TaskID retornado pelo agente (Working)
	OriginalMsg             string                   // mensagem original da primeira interação
	CurrentMsg              string                   // mensagem atual (primeira interação = OriginalMsg; continuação = resposta do usuário)
}

// Store define o contrato para armazenar e recuperar estado de sessão (Redis).
type Store interface {
	Get(clientID string) (*State, error)
	Set(clientID string, state *State) error
	GetOrCreate(clientID string) (*State, error)
}
