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
	"agent-client-go/internal/redis"
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

	redis.New()
	defer redis.Close()

	sqsClient, err := sqsclient.NewAWSSQS(ctx)
	if err != nil {
		log.Fatalf("falha ao criar cliente SQS: %v", err)
	}

	log.Printf("sessões em Redis em %s", appconfig.RedisAddr())
	orch := orchestrator.New(sqsClient)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	orch.Start(ctx)
	<-ctx.Done()
}
