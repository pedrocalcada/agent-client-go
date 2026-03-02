package planner

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
)

// BuildSystemPrompt monta o prompt do planejador com a lista de agentes configurados (ex.: config.yaml agent_urls).
func BuildSystemPrompt(agentNames []string) string {
	names := make([]string, len(agentNames))
	copy(names, agentNames)
	sort.Strings(names)

	var agentsLine string
	if len(names) == 0 {
		agentsLine = "Nenhum agente configurado. Use \"agente-nao-definido\" para qualquer tarefa."
	} else {
		lines := make([]string, len(names))
		for i, n := range names {
			lines[i] = "- " + n
		}
		agentsLine = "Agentes disponíveis (use exatamente um destes nomes no campo agent):\n" + strings.Join(lines, "\n")
	}

	return `Você é um planejador de tarefas. Dada a mensagem do usuário, quebre em UMA tarefa por ação, na ordem em que aparecem.

Regra importante: cada ação distinta vira uma tarefa, mesmo que seja o mesmo assunto. Exemplos:
- "fazer pix para minha mãe e pix para meu irmão" = 2 tarefas (duas mensagens para agente-pagamentos).
- "resgatar 1000 reais e fazer um pix de 500 e outro pix de 500" = 3 tarefas: 1 resgate (investimento) + 2 pix (pagamentos).

` + agentsLine + `

Caso não tenha agente para a tarefa, retorne o campo agent como "agente-nao-definido".

Responda SOMENTE com um JSON neste formato (sem texto antes ou depois):
{"tasks": [{"order": 1, "agent": "nome-do-agente", "message": "mensagem para o agente"}, ...]}`
}

// TaskItem representa uma tarefa retornada pelo planner.
type TaskItem struct {
	Order   int        `json:"order"`
	Agent   string     `json:"agent"`
	Message string     `json:"message"`
	TaskID  a2a.TaskID `json:"task_id"`
}

// ParseResponse extrai o JSON de tasks do conteúdo retornado pelo modelo (ex.: texto com ```json...```).
func ParseResponse(content string) ([]TaskItem, error) {
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
