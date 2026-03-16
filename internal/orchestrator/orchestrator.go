package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"

	appconfig "agent-client-go/internal/config"
	"agent-client-go/internal/llm"
	"agent-client-go/internal/message"
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
	callbackServer *CallbackServer
	planCh         chan *session.State
	resultsCh      chan *a2a.Task
}

// New cria o orquestrador. Sessões usam o client Redis global (redis).
func New(sqs sqsclient.SQSClient) *Orchestrator {
	o := &Orchestrator{
		sqs:       sqs,
		a2aPool:   NewAgentClientPool(appconfig.AgentURLs()),
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
				clientID, text := message.Parse(body)

				state, err := session.GetOrCreateSession(ctx, clientID)
				if err != nil {
					log.Printf("erro ao obter/criar sessão %s: %v", clientID, err)
					continue
				}
				if state.InConversationWithAgent != "" {
					state.CurrentMsg = text
				} else {
					state, err = planFromMessage(ctx, o, text, state)
					if err != nil {
						log.Printf("erro ao planejar via LLM: %v", err)
						continue
					}
				}
				if err := session.SetSession(ctx, clientID, state); err != nil {
					log.Printf("erro ao persistir sessão %s: %v", clientID, err)
					continue
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
}

func (o *Orchestrator) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case state := <-o.planCh:
			var plannedTask *a2a.Task
			var messageText string
			if state.InConversationWithAgent != "" {
				var err error
				plannedTask, err = GetPlannedTask(ctx, state.InConversationWithAgent)
				if err != nil {
					log.Printf("erro ao obter tarefa %s: %v", state.InConversationWithAgent, err)
					continue
				}
				messageText = state.CurrentMsg
			} else {
				for _, plannedItem := range state.TaskItems {
					task, err := GetPlannedTask(ctx, plannedItem.TaskID)
					if err != nil {
						log.Printf("erro ao obter tarefa %s: %v", plannedItem.TaskID, err)
						continue
					}
					if task.Status.State == a2a.TaskStateSubmitted {
						plannedTask = task
						messageText = plannedItem.Message
						break
					}
				}
			}
			workingTask, err := o.a2aPool.SendToAgent(ctx, plannedTask, messageText)
			if err != nil {
				plannedTask.Status.State = a2a.TaskStateFailed
				if err := SetPlannedTask(ctx, plannedTask); err != nil {
					log.Printf("erro ao persistir tarefa %s: %v", workingTask.ID, err)
				} else {
					o.resultsCh <- workingTask
				}
			}

		case task := <-o.resultsCh:

			if err := SetPlannedTask(ctx, task); err != nil {
				log.Printf("erro ao persistir tarefa %s: %v", task.ID, err)
				continue
			}
			state, err := session.GetSession(ctx, task.ContextID)
			if err != nil {
				log.Printf("erro ao obter sessão %s: %v", task.ContextID, err)
				continue
			}
			if state == nil {
				log.Printf("sessão não encontrada para context_id %s", task.ContextID)
				continue
			}

			if task.Status.State == a2a.TaskStateInputRequired {
				state.InConversationWithAgent = task.ID
				_ = session.SetSession(ctx, state.ClientID, state)
				o.sendResponseToClient(ctx, task.ContextID, textFromTask(task))
				continue
			}

			if task.Status.State == a2a.TaskStateCompleted {
				state.InConversationWithAgent = ""
				state.CompletedTasks[task.ID] = state.TaskItems[task.ID]
				delete(state.TaskItems, task.ID)
				_ = session.SetSession(ctx, state.ClientID, state)

			}

			if task.Status.State == a2a.TaskStateFailed {
				state.InConversationWithAgent = task.ID
				_ = session.SetSession(ctx, state.ClientID, state)
				o.sendResponseToClient(ctx, task.ContextID, textFromTask(task))
				continue
			}

			if len(state.TaskItems) > 0 {
				o.planCh <- state
				continue
			} else {
				o.consolidateAndRespond(ctx, state)
			}

		}
	}
}

func (o *Orchestrator) consolidateAndRespond(ctx context.Context, state *session.State) {
	var lines []string
	lines = append(lines, "Resultado das tarefas:")
	for _, item := range state.CompletedTasks {
		task, err := GetPlannedTask(ctx, item.TaskID)
		if err != nil {
			lines = append(lines, fmt.Sprintf("- falha ao obter tarefa %s: %v", item.TaskID, err))
			continue
		}
		if task.Status.State == a2a.TaskStateFailed {
			lines = append(lines, fmt.Sprintf("- falha na tarefa %s: %s", item.TaskID, textFromTask(task)))
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s", textFromTask(task)))
	}

	systemPrompt := planner.BuildSystemPromptForConsolidation(lines)
	content, err := llm.Call(ctx, systemPrompt, "")
	if err != nil {
		log.Printf("erro ao chamar LLM: %v", err)
		return
	}
	o.sendResponseToClient(ctx, state.ClientID, content)
}

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
	agents := o.a2aPool.GetPlannerAgents()
	systemPrompt := planner.BuildSystemPrompt(agents)
	content, err := llm.Call(ctx, systemPrompt, message)
	if err != nil {
		return nil, err
	}
	tasks, err := planner.ParseResponse(ctx, state.ClientID, content)
	if err != nil {
		return nil, err
	}
	return buildSessionState(ctx, tasks, state)
}

func buildSessionState(ctx context.Context, tasks []planner.TaskItem, state *session.State) (*session.State, error) {
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Order < tasks[j].Order })
	state.TaskItems = make(map[a2a.TaskID]planner.TaskItem)
	state.CompletedTasks = make(map[a2a.TaskID]planner.TaskItem)
	for _, task := range tasks {
		plannedTask := NewPlannedTask(ctx, state.ClientID, task.Agent)
		task.TaskID = plannedTask.ID
		state.TaskItems[plannedTask.ID] = task
	}
	return state, nil
}
