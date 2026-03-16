package redis

import (
	"context"
	"log"
	"sync"
	"time"

	appconfig "agent-client-go/internal/config"

	"github.com/redis/go-redis/v9"
)

var (
	client   *redis.Client
	initOnce sync.Once
)

// New inicializa o cliente Redis global do pacote (via sync.Once).
// Lê addr, password e db do appconfig (variável global). Deve ser chamado na main após LoadConfig().
// REDIS_ADDR é obrigatório; em caso de addr vazio ou falha ao conectar (ping), chama log.Fatal.
func New() {
	addr := appconfig.RedisAddr()
	if addr == "" {
		log.Fatal("REDIS_ADDR é obrigatório no config")
	}
	initOnce.Do(func() {
		c := redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: appconfig.RedisPassword(),
			DB:       appconfig.RedisDB(),
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Ping(ctx).Err(); err != nil {
			_ = c.Close()
			log.Fatalf("falha ao conectar ao Redis: %v", err)
		}
		client = c
	})
}

// Client retorna o cliente Redis global (sempre inicializado após New() com sucesso).
func Client() *redis.Client {
	return client
}

// Close fecha a conexão com o Redis. Deve ser chamado ao encerrar a aplicação (ex.: defer no main).
func Close() error {
	if client == nil {
		return nil
	}
	return client.Close()
}
