package main

import (
	"context"
	"log"

	appconfig "agent-client-go/internal/config"
	"agent-client-go/internal/llm"
	"agent-client-go/internal/orchestrator"
	"agent-client-go/internal/sqsclient"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := appconfig.LoadConfig()
	if err != nil {
		log.Fatalf("config inválida: %v", err)
	}

	llm.New()

	sqsClient, err := sqsclient.NewAWSSQS(ctx)
	if err != nil {
		log.Fatalf("falha ao criar cliente SQS: %v", err)
	}

	orch := orchestrator.New(sqsClient)
	orch.Start(ctx)

	select {}
}
