package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	appconfig "agent-client-go/internal/config"
	"agent-client-go/internal/llm"
	"agent-client-go/internal/orchestrator"
	"agent-client-go/internal/redisclient"
	"agent-client-go/internal/session"
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

	redisclient.New()
	defer redisclient.Close()

	sqsClient, err := sqsclient.NewAWSSQS(ctx)
	if err != nil {
		log.Fatalf("falha ao criar cliente SQS: %v", err)
	}

	store := session.NewRedisStore()
	log.Printf("SessionStore usando Redis em %s", appconfig.RedisAddr())

	orch := orchestrator.New(sqsClient, store)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	orch.Start(ctx)
	<-ctx.Done()
}
