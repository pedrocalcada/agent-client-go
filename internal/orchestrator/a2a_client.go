package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

// AgentClientPool mantém clientes A2A por agente (resolvidos via AgentCard) e envia
// mensagens usando os campos do protocolo (contextId, state, etc.).
type AgentClientPool struct {
	agentURLs map[string]string // nome do agente -> URL base do AgentCard
	resolver  *agentcard.Resolver
	clients   map[string]*a2aclient.Client
	mu        sync.Mutex
}

// NewAgentClientPool cria um pool que resolve AgentCard a partir das URLs configuradas.
func NewAgentClientPool(agentURLs map[string]string) *AgentClientPool {
	if agentURLs == nil {
		agentURLs = make(map[string]string)
	}
	return &AgentClientPool{
		agentURLs: agentURLs,
		resolver:  agentcard.DefaultResolver,
		clients:   make(map[string]*a2aclient.Client),
	}
}

// getAgentNames retorna os nomes dos agentes configurados (ordenados), para uso no planner.
func (p *AgentClientPool) getAgentNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	names := make([]string, 0, len(p.agentURLs))
	for name := range p.agentURLs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (p *AgentClientPool) getClient(ctx context.Context, agentName string) (*a2aclient.Client, error) {
	baseURL, ok := p.agentURLs[agentName]
	if !ok || baseURL == "" {
		return nil, fmt.Errorf("agente %q não possui URL A2A configurada", agentName)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[agentName]; ok {
		return c, nil
	}

	card, err := p.resolver.Resolve(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("resolver AgentCard para %s: %w", agentName, err)
	}

	client, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, fmt.Errorf("criar cliente A2A para %s: %w", agentName, err)
	}

	p.clients[agentName] = client
	return client, nil
}

// SendToAgent envia a tarefa ao agente. callbackURL é obrigatório; o agente responde Working e envia o resultado no callback.
// Em sucesso retorna a Task em estado Working para o orquestrador atualizar a task do planner.
func (p *AgentClientPool) SendToAgent(ctx context.Context, task *a2a.Task, callbackURL string) (*a2a.Task, error) {
	if callbackURL == "" {
		return nil, fmt.Errorf("callbackURL é obrigatório")
	}
	agentName := AgentFromTask(task)
	messageText := MessageTextFromPlannedTask(task)
	log.Printf("A2A: enviando para %s tarefa %s (context %s): %s", agentName, task.ID, task.ContextID, messageText)

	var msg *a2a.Message
	if task.Status.State == a2a.TaskStateSubmitted {
		msg = a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: messageText})
	} else {
		msg = a2a.NewMessageForTask(a2a.MessageRoleUser, task, a2a.TextPart{Text: messageText})
	}
	if msg.Metadata == nil {
		msg.Metadata = make(map[string]any)
	}

	msg.ContextID = task.ContextID
	msg.Metadata["requestId"] = task.ContextID
	msg.Metadata["callback_url"] = callbackURL

	client, err := p.getClient(ctx, agentName)
	if err != nil {
		return nil, err
	}
	params := &a2a.MessageSendParams{Message: msg}
	resp, err := client.SendMessage(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("enviar mensagem A2A para %s: %w", agentName, err)
	}

	return checkWorkingResponse(resp)
}

// checkWorkingResponse verifica se a resposta é Task em estado Working; retorna a Task ou erro.
func checkWorkingResponse(result a2a.SendMessageResult) (*a2a.Task, error) {
	task, ok := result.(*a2a.Task)
	if !ok {
		return nil, fmt.Errorf("resposta A2A inesperada: %T (esperado Task Working)", result)
	}
	if task.Status.State != a2a.TaskStateWorking {
		return nil, fmt.Errorf("task A2A em estado inesperado: %s (esperado Working)", task.Status.State)
	}
	return task, nil
}

func textFromParts(parts a2a.ContentParts) string {
	var out []string
	for _, p := range parts {
		if t, ok := p.(a2a.TextPart); ok {
			out = append(out, t.Text)
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n")
}

func textFromTask(t *a2a.Task) string {
	// Resposta pode estar em Status.Message (ex.: quando o agente retorna InputRequired).
	if t.Status.Message != nil && len(t.Status.Message.Parts) > 0 {
		if s := textFromParts(t.Status.Message.Parts); s != "" {
			return s
		}
	}
	// Última mensagem do agente no histórico
	for i := len(t.History) - 1; i >= 0; i-- {
		m := t.History[i]
		if m.Role == a2a.MessageRoleAgent && len(m.Parts) > 0 {
			return textFromParts(m.Parts)
		}
	}
	// Ou primeiro artefato com texto
	for _, art := range t.Artifacts {
		if art != nil && len(art.Parts) > 0 {
			return textFromParts(art.Parts)
		}
	}
	return ""
}
