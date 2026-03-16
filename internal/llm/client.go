package llm

import (
	appconfig "agent-client-go/internal/config"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

const defaultTimeout = 60 * time.Second

var (
	httpClient   *http.Client
	openaiClient *openai.Client
	initOnce     sync.Once
)

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	Think    bool      `json:"think"`
	Stream   bool      `json:"stream"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type response struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// New inicializa o cliente HTTP global do pacote (via sync.Once) e, se LLM_USE_OPENAI_SDK for true, o cliente do SDK OpenAI. Deve ser chamado na main.
func New() {
	initOnce.Do(func() {
		httpClient = &http.Client{Timeout: defaultTimeout}
		if appconfig.GetBool("LLM_USE_OPENAI_SDK") {
			opts := []option.RequestOption{}
			if key := appconfig.GetString("OPENAI_API_KEY"); key != "" {
				opts = append(opts, option.WithAPIKey(key))
			}
			c := openai.NewClient(opts...)
			openaiClient = &c
		}
	})
}

// Call envia system e user para a API do modelo e retorna o conteúdo da resposta (message.content).
// Se LLM_USE_OPENAI_SDK for true, usa o SDK oficial da OpenAI; caso contrário, usa a requisição HTTP para LLM_API_URL.
func Call(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if appconfig.GetBool("LLM_USE_OPENAI_SDK") {
		return callOpenAISDK(ctx, systemPrompt, userMessage)
	}
	return callHTTP(ctx, systemPrompt, userMessage)
}

func callOpenAISDK(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	if openaiClient == nil {
		return "", fmt.Errorf("cliente OpenAI não inicializado (LLM_USE_OPENAI_SDK=true)")
	}
	model := appconfig.GetString("OPENAI_MODEL")
	if model == "" {
		model = string(shared.ChatModelGPT4o)
	}
	chatResp, err := openaiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: shared.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.DeveloperMessage(systemPrompt),
			openai.UserMessage(userMessage),
		},
	})
	if err != nil {
		return "", fmt.Errorf("OpenAI SDK: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI SDK: resposta sem choices")
	}
	content := chatResp.Choices[0].Message.Content
	log.Printf("LLM resposta (OpenAI): %s", content)
	return content, nil
}

func callHTTP(ctx context.Context, systemPrompt, userMessage string) (string, error) {
	body := request{
		Model: appconfig.GetString("LLM_MODEL"),
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Think:  false,
		Stream: false,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, appconfig.GetString("LLM_API_URL"), bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API status %d", resp.StatusCode)
	}

	var apiResp response
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", err
	}
	log.Printf("LLM resposta: %s", apiResp.Message.Content)
	return apiResp.Message.Content, nil
}
