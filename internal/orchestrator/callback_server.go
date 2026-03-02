package orchestrator

import (
	appconfig "agent-client-go/internal/config"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/a2aproject/a2a-go/a2a"
)

// CallbackServer é o servidor HTTP que expõe o endpoint para o agente retornar a resposta ao orquestrador.
type CallbackServer struct {
	listenAddr string
	submit     func(*a2a.Task) // envia o Task A2A para o orquestrador (ex.: resultsCh)
	mu         sync.Mutex
	server     *http.Server
}

// NewCallbackServer cria o servidor de callback. listenAddr ex.: ":8080". submit é chamado quando o agente envia o evento A2A (Task).
func NewCallbackServer(submit func(*a2a.Task)) *CallbackServer {
	return &CallbackServer{
		listenAddr: appconfig.CallbackListenAddr(),
		submit:     submit,
	}
}

// Start inicia o servidor HTTP em background. Retorna assim que o listener está ativo.
func (s *CallbackServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/a2a/callback", s.handleCallback)
	s.mu.Lock()
	s.server = &http.Server{Addr: s.listenAddr, Handler: mux}
	s.mu.Unlock()
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("callback server erro: %v", err)
		}
	}()
	return nil
}

func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("callback: falha ao ler body: %v", err)
		http.Error(w, "falha ao ler body", http.StatusBadRequest)
		return
	}

	event, err := a2a.UnmarshalEventJSON(data)
	if err != nil {
		log.Printf("callback: body não é um Event A2A válido: %v", err)
		http.Error(w, "body não é um Event A2A válido", http.StatusBadRequest)
		return
	}

	switch ev := event.(type) {
	case *a2a.Task:
		s.submit(ev)
	default:
		// Por enquanto aceitamos apenas eventos de Task completos.
		http.Error(w, "apenas eventos A2A do tipo 'task' são suportados neste callback", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}
