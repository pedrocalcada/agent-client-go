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
)

const defaultTimeout = 60 * time.Second

var (
	httpClient *http.Client
	initOnce   sync.Once
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

// New inicializa o cliente HTTP global do pacote (via sync.Once). Deve ser chamado na main.
func New() {
	initOnce.Do(func() {
		httpClient = &http.Client{Timeout: defaultTimeout}
	})
}

// Call envia system e user para a API do modelo e retorna o conteúdo da resposta (message.content).
func Call(ctx context.Context, systemPrompt, userMessage string) (string, error) {
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
