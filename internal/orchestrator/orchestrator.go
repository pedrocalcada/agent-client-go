package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"

	appconfig "agent-client-go/internal/config"
	"agent-client-go/internal/llm"
	"agent-client-go/internal/planner"
	"agent-client-go/internal/session"
	"agent-client-go/internal/sqsclient"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type Orchestrator struct {
	sqs            sqsclient.SQSClient
	a2aPool        *AgentClientPool
	sessions       session.Store
	callbackServer *CallbackServer
	planCh         chan *session.State
	resultsCh      chan *a2a.Task
}

// New cria o orquestrador. O store deve ser session.NewRedisStore() (Redis obrigatório).
func New(sqs sqsclient.SQSClient, store session.Store) *Orchestrator {
	o := &Orchestrator{
		sqs:       sqs,
		a2aPool:   NewAgentClientPool(appconfig.AgentURLs()),
		sessions:  store,
		planCh:    make(chan *session.State, 16),
		resultsCh: make(chan *a2a.Task, 32),
	}
	o.callbackServer = NewCallbackServer(func(t *a2a.Task) {
		o.resultsCh <- t
	})
	return o
}

func (o *Orchestrator) Start(ctx context.Context) {
	if err := o.callbackServer.Start(); err != nil {
		log.Fatalf("iniciar servidor de callback: %v", err)
	}
	log.Printf("callback do orquestrador (agentes enviam resultado para %s/a2a/callback)", appconfig.GetString("ORCHESTRATOR_CALLBACK_BASE_URL"))
	go o.consumeInput(ctx)
	go o.run(ctx)
}

func (o *Orchestrator) consumeInput(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := o.sqs.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(appconfig.GetString("SQS_INPUT_QUEUE_URL")),
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     5,
		})
		if err != nil {
			log.Printf("erro ao receber mensagem: %v", err)
			continue
		}
		for _, msg := range resp.Messages {
			body := aws.ToString(msg.Body)
			clientID, text := session.ParseIncomingMessage(body)

			state, err := o.sessions.GetOrCreate(clientID)
			if err != nil {
				log.Printf("erro ao obter/criar sessão %s: %v", clientID, err)
				continue
			}
			if state.InConversationWithAgent != "" {
				state.CurrentMsg = text
				_ = o.sessions.Set(clientID, state)
			} else {
				state, err = planFromMessage(ctx, o, text, state)
				if err != nil {
					log.Printf("erro ao planejar via LLM: %v", err)
					continue
				}
				_ = o.sessions.Set(clientID, state)
			}
			o.planCh <- state
			if _, err := o.sqs.DeleteMessage(ctx, &sqs.DeleteMessageInput{
				QueueUrl:      aws.String(appconfig.GetString("SQS_INPUT_QUEUE_URL")),
				ReceiptHandle: msg.ReceiptHandle,
			}); err != nil {
				log.Printf("erro ao deletar mensagem %s: %v", aws.ToString(msg.MessageId), err)
			}
		}
	}
}

func (o *Orchestrator) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case state := <-o.planCh:
			var plannedTask *a2a.Task

			if state.InConversationWithAgent != "" {
				plannedTask = state.Tasks[state.InConversationWithAgent]
				plannedTask.History = append(plannedTask.History, &a2a.Message{
					ID:    a2a.NewMessageID(),
					Role:  a2a.MessageRoleUser,
					Parts: a2a.ContentParts{a2a.TextPart{Text: state.CurrentMsg}},
				})
			} else {
				plannedItem := state.TaskItems[len(state.Tasks)]
				plannedTask = NewPlannedTask(state.SessionID, plannedItem.Agent, strings.TrimSpace(plannedItem.Message))
			}

			workingTask, err := o.callAgentA2A(ctx, plannedTask)
			if err != nil {
				workingTask = taskFromAgentError(plannedTask, err)
				state.Tasks[plannedTask.ID] = plannedTask
				_ = o.sessions.Set(state.ClientID, state)
				o.resultsCh <- workingTask
				continue
			}
			UpdateTaskFromWorking(plannedTask, workingTask)
			state.TaskItems[len(state.Tasks)].TaskID = plannedTask.ID
			state.Tasks[plannedTask.ID] = plannedTask
			_ = o.sessions.Set(state.ClientID, state)

		case task := <-o.resultsCh:

			state, err := o.sessions.Get(task.ContextID)
			if err != nil {
				log.Printf("erro ao obter sessão %s: %v", task.ContextID, err)
				continue
			}
			if state == nil {
				log.Printf("sessão não encontrada para context_id %s", task.ContextID)
				continue
			}
			UpdateTaskFromWorking(state.Tasks[task.ID], task)

			if task.Status.State == a2a.TaskStateInputRequired {
				state.InConversationWithAgent = task.ID
				_ = o.sessions.Set(state.ClientID, state)
				o.sendResponseToClient(ctx, task.ContextID, textFromTask(task))
				continue
			}

			if task.Status.State == a2a.TaskStateCompleted && len(state.Tasks) < len(state.TaskItems) {
				state.InConversationWithAgent = ""
				_ = o.sessions.Set(state.ClientID, state)
				o.planCh <- state
				continue
			}

			if task.Status.State == a2a.TaskStateCompleted && len(state.Tasks) == len(state.TaskItems) {
				state.InConversationWithAgent = ""
				_ = o.sessions.Set(state.ClientID, state)
				o.consolidateAndRespond(ctx, state)
			}

			if task.Status.State == a2a.TaskStateFailed {
				state.InConversationWithAgent = ""
				_ = o.sessions.Set(state.ClientID, state)
				o.consolidateAndRespond(ctx, state)
			}

		}
	}
}

// taskFromAgentError monta um *a2a.Task em estado Failed para quando o envio ao agente falha.
func taskFromAgentError(planned *a2a.Task, err error) *a2a.Task {
	log.Printf("erro ao executar a tarefa: %v", err)
	planned.History = append(planned.History, &a2a.Message{
		ID:    a2a.NewMessageID(),
		Role:  a2a.MessageRoleAgent,
		Parts: a2a.ContentParts{a2a.TextPart{Text: "Erro ao executar a tarefa"}},
	})
	return planned
}

// callAgentA2A envia a tarefa (a2a.Task) ao agente via A2A. Retorna a Task em estado Working para atualizar o planner.
func (o *Orchestrator) callAgentA2A(ctx context.Context, task *a2a.Task) (*a2a.Task, error) {
	return o.a2aPool.SendToAgent(ctx, task, appconfig.CallbackBaseURL())
}

func (o *Orchestrator) consolidateAndRespond(ctx context.Context, state *session.State) {
	var lines []string
	lines = append(lines, "Resultado das tarefas:")
	for _, t := range state.Tasks {
		if t.Status.State == a2a.TaskStateFailed {
			lines = append(lines, fmt.Sprintf("- falha na tarefa %s: %s", t.ID, textFromTask(t)))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s", textFromTask(t)))
	}
	body := mustJSON(struct {
		IdCliente string `json:"id_cliente"`
		Resposta  string `json:"resposta"`
	}{state.SessionID, strings.Join(lines, "\n")})
	if _, err := o.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(appconfig.GetString("SQS_OUTPUT_QUEUE_URL")),
		MessageBody: aws.String(body),
	}); err != nil {
		log.Printf("erro ao enviar resposta: %v", err)
	}
}

// sendResponseToClient envia uma única resposta ao cliente (ex.: quando o agente está em modo conversa).
func (o *Orchestrator) sendResponseToClient(ctx context.Context, clientID string, response string) {
	body := mustJSON(struct {
		IdCliente string `json:"id_cliente"`
		Resposta  string `json:"resposta"`
	}{clientID, response})
	if _, err := o.sqs.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(appconfig.GetString("SQS_OUTPUT_QUEUE_URL")),
		MessageBody: aws.String(body),
	}); err != nil {
		log.Printf("erro ao enviar resposta ao cliente: %v", err)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func planFromMessage(ctx context.Context, o *Orchestrator, message string, state *session.State) (*session.State, error) {
	message = strings.TrimSpace(message)
	agentNames := o.a2aPool.getAgentNames()
	systemPrompt := planner.BuildSystemPrompt(agentNames)
	content, err := llm.Call(ctx, systemPrompt, message)
	if err != nil {
		return nil, err
	}
	tasks, err := planner.ParseResponse(content)
	if err != nil {
		return nil, err
	}
	return buildSessionState(message, tasks, state)
}

func buildSessionState(originalMsg string, tasks []planner.TaskItem, state *session.State) (*session.State, error) {
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Order < tasks[j].Order })
	state.OriginalMsg = originalMsg
	state.CurrentMsg = originalMsg
	state.TaskItems = tasks
	state.Tasks = make(map[a2a.TaskID]*a2a.Task)
	return state, nil
}
