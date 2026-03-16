package message

import (
	"encoding/json"
	"strings"
)

// Parse interpreta o body do SQS. Se for JSON com id_cliente e mensagem/texto, retorna clientID e texto;
// senão trata o body inteiro como texto e clientID fica vazio.
func Parse(body string) (clientID string, text string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", ""
	}
	var m struct {
		ClientID string `json:"id_cliente"`
		Text     string `json:"mensagem"`
		TextAlt  string `json:"texto"`
	}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return "", body
	}
	msg := m.Text
	if msg == "" {
		msg = m.TextAlt
	}
	return m.ClientID, msg
}
