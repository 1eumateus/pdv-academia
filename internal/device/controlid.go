// Caminho: catraca/internal/device/controlid.go

// Package device conversa com o hardware: o leitor facial Control iD (iDFace)
// pela API HTTP dele e a placa de relé ligada na porta serial (catraca
// eletromecânica). Não tem regra de negócio: quem decide é o pacote access.
package device

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// User é um usuário cadastrado no iDFace.
type User struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Registration string `json:"registration"`
}

// ControlID é o cliente da API do iDFace (login.fcgi + sessão).
type ControlID struct {
	Host     string // ip ou ip:porta
	Login    string
	Password string
	SecBoxID int64 // 0 = relé interno do aparelho (door=1)

	mu      sync.Mutex
	session string
	http    http.Client
}

func NewControlID(host, login, password string, secBox int64) *ControlID {
	return &ControlID{Host: host, Login: login, Password: password, SecBoxID: secBox,
		http: http.Client{Timeout: 90 * time.Second}}
}

var errSession = errors.New("sessão do iDFace expirada")

func (c *ControlID) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+c.Host+"/"+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("o iDFace em %s não respondeu (confira o IP e a rede)", c.Host)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return errSession
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("o iDFace respondeu %d: %s", res.StatusCode, bytes.TrimSpace(data))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// call faz login quando preciso e repete uma vez se a sessão expirou.
func (c *ControlID) call(ctx context.Context, path string, in, out any) error {
	return c.withSession(ctx, func(session string) error { return c.post(ctx, path+"?session="+session, in, out) })
}

func (c *ControlID) withSession(ctx context.Context, fn func(session string) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for try := 0; ; try++ {
		if c.session == "" {
			var r struct{ Session string }
			if err := c.post(ctx, "login.fcgi", map[string]string{"login": c.Login, "password": c.Password}, &r); err != nil {
				if errors.Is(err, errSession) {
					return errors.New("usuário ou senha do iDFace incorretos")
				}
				return err
			}
			c.session = r.Session
		}
		err := fn(c.session)
		if errors.Is(err, errSession) && try == 0 {
			c.session = ""
			continue
		}
		return err
	}
}

// ReleaseAction é a ação que abre a catraca, no formato da API.
func (c *ControlID) ReleaseAction(reason int) map[string]string {
	if c.SecBoxID == 0 {
		return map[string]string{"action": "door", "parameters": "door=1"}
	}
	return map[string]string{"action": "sec_box", "parameters": fmt.Sprintf("id=%d, reason=%d", c.SecBoxID, reason)}
}

// Release libera um giro da catraca (motivo 3 = liberação pelo sistema).
func (c *ControlID) Release(ctx context.Context) error {
	return c.call(ctx, "execute_actions.fcgi", map[string]any{"actions": []any{c.ReleaseAction(3)}}, nil)
}

// Users lista os usuários cadastrados no aparelho.
func (c *ControlID) Users(ctx context.Context) ([]User, error) {
	var r struct{ Users []User }
	err := c.call(ctx, "load_objects.fcgi", map[string]any{"object": "users"}, &r)
	return r.Users, err
}

// CreateUser cadastra a pessoa no aparelho e devolve o id dela lá.
func (c *ControlID) CreateUser(ctx context.Context, name, registration string) (int64, error) {
	var r struct{ IDs []int64 }
	err := c.call(ctx, "create_objects.fcgi", map[string]any{"object": "users",
		"values": []any{map[string]string{"name": name, "registration": registration}}}, &r)
	if err == nil && len(r.IDs) != 1 {
		err = errors.New("o iDFace não devolveu o id do usuário criado")
	}
	if err != nil {
		return 0, err
	}
	return r.IDs[0], nil
}

// DeleteUser apaga a pessoa (e o rosto) do aparelho.
func (c *ControlID) DeleteUser(ctx context.Context, id int64) error {
	return c.call(ctx, "destroy_objects.fcgi", map[string]any{"object": "users",
		"where": map[string]any{"users": map[string]any{"id": id}}}, nil)
}

// EnrollFace abre a câmera do aparelho para cadastrar o rosto; volta quando a
// pessoa termina (ou desiste) na frente do leitor. auto: com o rosto
// centralizado, o aparelho conta 3 s e confirma a foto sozinho (sem tocar no display).
func (c *ControlID) EnrollFace(ctx context.Context, userID int64) error {
	var r struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
	}
	err := c.call(ctx, "remote_enroll.fcgi", map[string]any{"type": "face", "user_id": userID, "save": true, "sync": true,
		"auto": true, "countdown": 3}, &r)
	if err == nil && r.Success != nil && !*r.Success {
		err = fmt.Errorf("o cadastro do rosto não foi concluído no aparelho %s", r.Error)
	}
	return err
}

// SetUserImage cadastra o rosto a partir de uma foto (JPEG ou PNG), em vez da
// câmera do aparelho. match=1: o aparelho recusa se o rosto já for de outra
// pessoa cadastrada. Se a foto não servir (rosto pequeno, torto, borrado), ele
// devolve o motivo.
func (c *ControlID) SetUserImage(ctx context.Context, userID int64, img []byte) error {
	return c.withSession(ctx, func(session string) error {
		url := fmt.Sprintf("http://%s/user_set_image.fcgi?user_id=%d&match=1&timestamp=%d&session=%s", c.Host, userID, time.Now().Unix(), session)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(img))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("o iDFace em %s não respondeu (confira o IP e a rede)", c.Host)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if res.StatusCode == http.StatusUnauthorized {
			return errSession
		}
		var r struct {
			Success *bool `json:"success"`
			Errors  []struct {
				Message string `json:"message"`
			} `json:"errors"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &r)
		if res.StatusCode != http.StatusOK || (r.Success != nil && !*r.Success) {
			msgs := []string{}
			for _, e := range r.Errors {
				if e.Message != "" {
					msgs = append(msgs, e.Message)
				}
			}
			if r.Error != "" {
				msgs = append(msgs, r.Error)
			}
			if len(msgs) == 0 {
				msgs = append(msgs, strings.TrimSpace(string(data)))
			}
			return fmt.Errorf("a foto não foi aceita pelo aparelho: %s", strings.Join(msgs, "; "))
		}
		return nil
	})
}

// UserImage devolve a foto (JPEG) que o aparelho guardou no cadastro do rosto;
// nil quando a pessoa não tem foto lá.
func (c *ControlID) UserImage(ctx context.Context, userID int64) ([]byte, error) {
	var img []byte
	err := c.withSession(ctx, func(session string) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("http://%s/user_get_image.fcgi?user_id=%d&session=%s", c.Host, userID, session), nil)
		if err != nil {
			return err
		}
		res, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("o iDFace em %s não respondeu (confira o IP e a rede)", c.Host)
		}
		defer res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized {
			return errSession
		}
		// Sem foto o aparelho responde com erro em JSON: não é falha.
		if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/") {
			img = nil
			return nil
		}
		img, err = io.ReadAll(io.LimitReader(res.Body, 4<<20))
		return err
	})
	return img, err
}
