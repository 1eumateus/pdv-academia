// Caminho: pdv/internal/license/license.go

// Package license assina e verifica licenças offline com Ed25519.
//
// Formato do texto: "PDV1.<dados>.<assinatura>", ambos em base64 URL sem
// preenchimento. Os dados são JSON (Data); a assinatura cobre "PDV1.<dados>".
// O app só conhece a chave pública (PublicKey); a privada fica com quem emite
// as licenças (cmd/licgen) e nunca entra no repositório nem no instalador.
package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const prefix = "PDV1."

// Data é o conteúdo assinado da licença.
type Data struct {
	Store     string `json:"loja"`
	Document  string `json:"documento"`
	MachineID string `json:"maquina"`
	// Expires: "AAAA-MM-DD" (vale até o fim desse dia); vazio = vitalícia.
	Expires  string `json:"validade,omitempty"`
	Plan     string `json:"plano"`
	IssuedAt string `json:"emitida"`
}

var ErrInvalid = errors.New("licença inválida ou adulterada")

var b64 = base64.RawURLEncoding

// Sign gera o texto da licença.
func Sign(priv ed25519.PrivateKey, d Data) (string, error) {
	if d.Expires != "" {
		if _, err := time.Parse("2006-01-02", d.Expires); err != nil {
			return "", errors.New("validade deve estar no formato AAAA-MM-DD")
		}
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	body := prefix + b64.EncodeToString(raw)
	return body + "." + b64.EncodeToString(ed25519.Sign(priv, []byte(body))), nil
}

// Verify confere a assinatura e devolve os dados. Espaços e quebras de linha
// (de copiar e colar de e-mail ou WhatsApp) são ignorados.
func Verify(pub ed25519.PublicKey, text string) (Data, error) {
	text = strings.Join(strings.Fields(text), "")
	if !strings.HasPrefix(text, prefix) {
		return Data{}, ErrInvalid
	}
	i := strings.LastIndexByte(text, '.')
	if i <= len(prefix) {
		return Data{}, ErrInvalid
	}
	body, sigText := text[:i], text[i+1:]
	sig, err := b64.DecodeString(sigText)
	if err != nil || !ed25519.Verify(pub, []byte(body), sig) {
		return Data{}, ErrInvalid
	}
	raw, err := b64.DecodeString(body[len(prefix):])
	if err != nil {
		return Data{}, ErrInvalid
	}
	var d Data
	if err := json.Unmarshal(raw, &d); err != nil {
		return Data{}, ErrInvalid
	}
	return d, nil
}

// ExpiresAt devolve o último instante de validade (fim do dia, no fuso local),
// ou nil se for vitalícia.
func (d Data) ExpiresAt() *time.Time {
	if d.Expires == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02", d.Expires, time.Local)
	if err != nil {
		return nil
	}
	end := t.AddDate(0, 0, 1).Add(-time.Second)
	return &end
}

// PublicKey é a chave que verifica as licenças (gerada por `licgen keygen`).
func PublicKey() ed25519.PublicKey {
	k, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(k) != ed25519.PublicKeySize {
		panic("license: chave pública embutida inválida")
	}
	return k
}
