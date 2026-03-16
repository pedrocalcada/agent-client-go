package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
)

// AgentInfo descreve um agente disponível para o planner.
// É construído a partir do AgentCard resolvido pelos clientes A2A.
type AgentInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Skills      []string `json:"skills,omitempty"`
}

// BuildSystemPrompt monta o prompt do planejador com a lista de agentes e seus detalhes (ex.: descrição e skills).
func BuildSystemPrompt(agents map[string]*a2a.AgentCard) string {

	var agentsLine string

	var lines []string
	for _, card := range agents {
		var b strings.Builder
		fmt.Fprintf(&b, "- %s", card.Name)
		if strings.TrimSpace(card.Description) != "" {
			fmt.Fprintf(&b, " — descrição: %s", strings.TrimSpace(card.Description))
		}
		if len(card.Skills) > 0 {
			fmt.Fprintf(&b, "\n  Skills principais:")
			for _, s := range card.Skills {
				if strings.TrimSpace(s.Description) == "" {
					continue
				}
				fmt.Fprintf(&b, "\n    - %s", strings.TrimSpace(s.Description))
			}
		}
		lines = append(lines, b.String())
	}
	agentsLine = "Agentes disponíveis (use exatamente um destes nomes no campo agent, escolhendo o mais adequado pela descrição e skills):\n" + strings.Join(lines, "\n")

	return `Você é um planejador de tarefas. Dada a mensagem do usuário, quebre em UMA tarefa por ação, na ordem em que aparecem. Garanta que todas as intenções do usuário sejam atendidas.

Regra importante: cada ação distinta vira uma tarefa, mesmo que seja o mesmo assunto. Exemplos:
- "fazer pix para minha mãe e pix para meu irmão" = 2 tarefas (duas mensagens para agente-pagamentos).
- "resgatar 1000 reais e fazer um pix de 500 e outro pix de 500" = 3 tarefas: 1 resgate (investimento) + 2 pix (pagamentos).

` + agentsLine + `

Caso não tenha agente para a tarefa, retorne o campo agent como "agente-nao-definido".

Responda SOMENTE com um JSON neste formato (sem texto antes ou depois):
{"tasks": [{"order": 1, "agent": "nome-do-agente", "message": "mensagem para o agente"}, ...]}`
}

func BuildSystemPromptForConsolidation(lines []string) string {
	return `Você é um planejador de tarefas e executou todas elas e teve essa troca de mensagens com o usuário.

` + strings.Join(lines, "\n") + `

Resuma com um texto amigável para o usuário. o que foi feito e o que não foi feito.`
}

// TaskItem representa uma tarefa retornada pelo planner.
type TaskItem struct {
	Order   int        `json:"order"`
	Agent   string     `json:"agent"`
	Message string     `json:"message"`
	TaskID  a2a.TaskID `json:"task_id"`
}

// ParseResponse extrai o JSON de tasks do conteúdo retornado pelo modelo (ex.: texto com ```json...```).
func ParseResponse(ctx context.Context, sessionID string, content string) ([]TaskItem, error) {
	content = strings.TrimSpace(content)
	if i := strings.Index(content, "{"); i >= 0 {
		content = content[i:]
	}
	if i := strings.LastIndex(content, "}"); i >= 0 {
		content = content[:i+1]
	}
	var out struct {
		Tasks []TaskItem `json:"tasks"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("parsear resposta do planner: %w", err)
	}
	return out.Tasks, nil
}
