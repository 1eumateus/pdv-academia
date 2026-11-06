// Caminho: catraca/internal/device/server.go

package device

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Decision é a resposta do sistema para uma pessoa reconhecida no aparelho.
type Decision struct {
	Allowed bool
	Name    string
	Message string // aparece na tela do iDFace
}

// OnNotice recebe os avisos do aparelho que não são reconhecimento de rosto
// (ex.: giro da catraca, porta, SecBox). Serve para diagnóstico: só registra.
var OnNotice func(path, body string)

// Decider decide o acesso do usuário do aparelho (deviceUserID).
type Decider func(ctx context.Context, deviceUserID int64) Decision

// Server atende o modo online do iDFace: a cada rosto reconhecido o aparelho
// chama /new_user_identified.fcgi e espera a resposta com as ações.
type Server struct {
	srv      *http.Server
	lastSeen atomic.Int64 // unix: última chamada do aparelho
}

// handler monta as rotas. allowActions devolve as ações que abrem a catraca
// no próprio aparelho (vazio quando quem abre é o relé serial).
func (s *Server) handler(decide Decider, allowActions func() []map[string]string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/new_user_identified.fcgi", func(w http.ResponseWriter, r *http.Request) {
		s.lastSeen.Store(time.Now().Unix())
		p := params(r)
		userID, _ := strconv.ParseInt(p.Get("user_id"), 10, 64)
		portal, _ := strconv.ParseInt(p.Get("portal_id"), 10, 64)
		res := map[string]any{"event": 3, "user_id": userID, "user_name": "", "user_image": false,
			"portal_id": portal, "actions": []any{}, "message": "Não identificado"}
		if userID > 0 {
			d := decide(r.Context(), userID)
			res["user_name"], res["message"] = d.Name, d.Message
			if d.Allowed {
				res["event"], res["actions"] = 7, allowActions()
			} else {
				res["event"] = 6
			}
		}
		// Diagnóstico do giro: a passagem entra no log para comparar com os avisos seguintes.
		if fn := OnNotice; fn != nil {
			fn("rosto reconhecido", fmt.Sprintf("usuario=%d portal=%d liberado=%v", userID, portal, res["event"] == 7))
		}
		writeJSON(w, map[string]any{"result": res})
	})
	// device_is_alive.fcgi e demais avisos do aparelho: só confirma. Os avisos
	// (menos o "estou vivo", que chega o tempo todo) vão para OnNotice.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.lastSeen.Store(time.Now().Unix())
		body, _ := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		io.Copy(io.Discard, io.LimitReader(r.Body, 8<<20))
		if fn := OnNotice; fn != nil && !strings.Contains(r.URL.Path, "device_is_alive") {
			fn(r.Method+" "+r.URL.RequestURI(), string(body))
		}
		writeJSON(w, map[string]any{})
	})
	return mux
}

// params junta os campos da URL, de formulário ou JSON (varia com o firmware).
func params(r *http.Request) url.Values {
	v := r.URL.Query()
	body, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	ct := r.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "json"):
		var m map[string]any
		if json.Unmarshal(body, &m) == nil {
			for k, x := range m {
				switch n := x.(type) {
				case float64:
					v.Set(k, strconv.FormatInt(int64(n), 10))
				case string:
					v.Set(k, n)
				}
			}
		}
	case strings.Contains(ct, "urlencoded"):
		if f, err := url.ParseQuery(string(body)); err == nil {
			for k := range f {
				v.Set(k, f.Get(k))
			}
		}
	}
	return v
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// Listen abre a porta e atende em segundo plano.
func Listen(port int, decide Decider, allowActions func() []map[string]string) (*Server, error) {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	s := &Server{}
	s.srv = &http.Server{Handler: s.handler(decide, allowActions), ReadHeaderTimeout: 10 * time.Second}
	go s.srv.Serve(ln)
	return s, nil
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if s.srv.Shutdown(ctx) != nil {
		s.srv.Close()
	}
}

// LastSeen: última vez que o aparelho chamou (zero se nunca).
func (s *Server) LastSeen() time.Time {
	if n := s.lastSeen.Load(); n > 0 {
		return time.Unix(n, 0)
	}
	return time.Time{}
}

// LocalIPs lista os IPv4 deste computador (para configurar no aparelho).
func LocalIPs() []string {
	out := []string{}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n.IP.String())
		}
	}
	return out
}
