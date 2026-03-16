package config

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
)

// globalV guarda a instância do viper após LoadConfig(); usado por GetString, GetStringMapString, etc.
var globalV *viper.Viper

func LoadConfig() (*viper.Viper, error) {
	v := viper.New()

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("falha ao ler config.yaml: %w", err)
	}

	if v.GetString("SQS_INPUT_QUEUE_URL") == "" || v.GetString("SQS_OUTPUT_QUEUE_URL") == "" {
		return nil, errors.New("defina SQS_INPUT_QUEUE_URL e SQS_OUTPUT_QUEUE_URL no config.yaml")
	}
	if !v.GetBool("LLM_USE_OPENAI_SDK") && v.GetString("LLM_API_URL") == "" {
		return nil, errors.New("defina LLM_API_URL no config.yaml (endpoint HTTP da API do modelo) ou use LLM_USE_OPENAI_SDK: true para o SDK OpenAI")
	}

	globalV = v
	return v, nil
}

// GetString retorna o valor string da chave de configuração. Pode ser chamado de qualquer lugar após LoadConfig().
func GetString(key string) string {
	if globalV == nil {
		return ""
	}
	return globalV.GetString(key)
}

// GetBool retorna o valor bool da chave de configuração. Pode ser chamado de qualquer lugar após LoadConfig().
func GetBool(key string) bool {
	if globalV == nil {
		return false
	}
	return globalV.GetBool(key)
}

// GetStringMapString retorna o mapa string->string da chave. Útil para configs como agent_urls.
func GetStringMapString(key string) map[string]string {
	if globalV == nil {
		return nil
	}
	return globalV.GetStringMapString(key)
}

// AgentURLs retorna o mapa nome-do-agente -> URL base do AgentCard A2A.
// Chaves esperadas: agente-investimento, agente-pagamentos, agente-geral (conforme planner).
func AgentURLs() map[string]string {
	out := make(map[string]string)
	if m := GetStringMapString("agent_urls"); len(m) > 0 {
		for k, u := range m {
			if u != "" {
				out[k] = u
			}
		}
	}
	for _, name := range []string{"agente-investimento", "agente-pagamentos", "agente-geral"} {
		key := "AGENT_URL_" + name
		if u := GetString(key); u != "" {
			out[name] = u
		}
	}
	return out
}

// CallbackListenAddr retorna o endereço em que o orquestrador escuta callbacks dos agentes (ex.: ":8080").
// Se vazio, o servidor de callback não é iniciado.
func CallbackListenAddr() string {
	return GetString("ORCHESTRATOR_CALLBACK_LISTEN")
}

// CallbackBaseURL retorna a URL base que o orquestrador informa aos agentes para devolver a resposta (ex.: "http://localhost:8080").
func CallbackBaseURL() string {
	return GetString("ORCHESTRATOR_CALLBACK_BASE_URL")
}

// RedisAddr retorna o endereço do Redis para o SessionStore (ex.: "localhost:6379"). Se vazio, usa store em memória.
func RedisAddr() string {
	return GetString("REDIS_ADDR")
}

// RedisPassword retorna a senha do Redis (opcional).
func RedisPassword() string {
	return GetString("REDIS_PASSWORD")
}

// RedisDB retorna o número do DB Redis (0 por padrão).
func RedisDB() int {
	if v := globalV; v != nil && v.IsSet("REDIS_DB") {
		return v.GetInt("REDIS_DB")
	}
	return 0
}
