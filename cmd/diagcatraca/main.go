// Caminho: catraca/cmd/diagcatraca/main.go

// diagcatraca: diagnóstico para descobrir se a catraca avisa quando a pessoa
// gira. Só LÊ: não abre a porta 8010, não muda a configuração do iDFace, não
// aciona a catraca e abre o banco do PDV em modo somente leitura. Pode rodar
// com o PDV aberto e a academia funcionando.
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const espera = 15 * time.Second // tempo para o giro depois da liberação

var (
	logf   *os.File
	client = http.Client{Timeout: 10 * time.Second}
)

func say(format string, a ...any) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, a...)
	fmt.Println(line)
	if logf != nil {
		fmt.Fprintln(logf, line)
	}
}

type liberacao struct {
	quem string
	at   time.Time
}

func main() {
	exe, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exe), "diagnostico-catraca-"+time.Now().Format("2006-01-02-150405")+".txt")
	logf, _ = os.Create(logPath)
	defer func() {
		if logf != nil {
			logf.Close()
		}
		fmt.Println("\nTecle Enter para fechar.")
		fmt.Scanln()
	}()

	say("Diagnóstico da catraca (só leitura, não interfere no PDV). Feche a janela para parar.")
	say("Registro salvo em %s", logPath)

	db, host, login, senha := abrirBanco()
	if db != nil {
		defer db.Close()
	}
	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	if host == "" {
		say("IP do iDFace não encontrado na configuração do PDV. Rode: diagcatraca.exe <ip-do-idface>")
		return
	}
	say("iDFace: %s", host)

	dev := &idface{host: host, login: login, senha: senha}
	if err := dev.entrar(); err != nil {
		say("ERRO ao entrar no iDFace: %v", err)
		return
	}
	say("Conectado ao iDFace. Passe na catraca (ou peça para alguém passar) e veja abaixo.")
	say("------------------------------------------------------------")

	var (
		pendentes   []liberacao
		ultPassagem int64 = -1
		ultLog      int64 = -1
		ultPortas   string
		erroPortas  bool
		erroLogs    bool
		girou, nao  int
	)
	for {
		agora := time.Now()

		// 1) Liberações gravadas pelo PDV (somente leitura).
		if db != nil {
			for _, p := range novasPassagens(db, &ultPassagem) {
				pendentes = append(pendentes, liberacao{p, agora})
				say("LIBERADO   %s  (aguardando giro por %ds...)", p, int(espera.Seconds()))
			}
		}

		// 2) Registro de acessos do próprio iDFace.
		if !erroLogs {
			rows, err := dev.logs(&ultLog)
			if err != nil {
				erroLogs = true
				say("aviso: não deu para ler o registro de acessos do iDFace (%v)", err)
			}
			for _, r := range rows {
				ev, _ := r["event"].(float64)
				switch int(ev) {
				case 13:
					say("DESISTIU   (iDFace registrou desistência) %s", compacto(r))
					if len(pendentes) > 0 {
						pendentes = pendentes[1:]
						nao++
					}
				case 7:
					if db == nil {
						pendentes = append(pendentes, liberacao{fmt.Sprintf("usuário %v do iDFace", r["user_id"]), agora})
						say("LIBERADO   usuário %v do iDFace (aguardando giro...)", r["user_id"])
					}
				default:
					say("iDFace     evento %d %s", int(ev), compacto(r))
				}
			}
		}

		// 3) Sensor de porta / SecBox / giro.
		if !erroPortas {
			st, err := dev.portas()
			switch {
			case err != nil:
				erroPortas = true
				say("aviso: o iDFace não informou o estado do sensor (%v)", err)
			case ultPortas == "":
				ultPortas = st
				say("Sensor agora: %s", st)
			case st != ultPortas:
				say("SENSOR     mudou: %s", st)
				ultPortas = st
				if len(pendentes) > 0 {
					say("  => GIROU: %s", pendentes[0].quem)
					pendentes = pendentes[1:]
					girou++
				}
			}
		}

		// 4) Liberações sem nenhum sinal dentro do tempo.
		for len(pendentes) > 0 && agora.Sub(pendentes[0].at) > espera {
			say("  => SEM SINAL DE GIRO em %ds: %s", int(espera.Seconds()), pendentes[0].quem)
			pendentes = pendentes[1:]
			nao++
		}
		if girou+nao > 0 && (girou+nao)%5 == 0 && len(pendentes) == 0 {
			say("Resumo até agora: %d com sinal de giro, %d sem sinal", girou, nao)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// ---------- banco do PDV (somente leitura) ----------

func abrirBanco() (*sql.DB, string, string, string) {
	login, senha := "admin", "admin"
	path := filepath.Join(os.Getenv("APPDATA"), "pdv", "pdv.db")
	if _, err := os.Stat(path); err != nil {
		say("Banco do PDV não encontrado em %s (vou usar só o iDFace)", path)
		return nil, "", login, senha
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro&_pragma=busy_timeout(3000)&_pragma=query_only(1)")
	if err != nil {
		say("Não abriu o banco do PDV: %v", err)
		return nil, "", login, senha
	}
	db.SetMaxOpenConns(1)
	var host string
	rows, err := db.Query(`SELECT key, value FROM settings WHERE key IN ('catraca_device_host','catraca_device_login','catraca_device_password','catraca_relay','catraca_secbox_id')`)
	if err != nil {
		say("Não leu a configuração do PDV: %v", err)
		return db, "", login, senha
	}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		switch k {
		case "catraca_device_host":
			host = v
		case "catraca_device_login":
			login = v
		case "catraca_device_password":
			senha = v
		case "catraca_relay":
			say("PDV: quem abre a catraca = %s", v)
		case "catraca_secbox_id":
			say("PDV: SecBox = %s", v)
		}
	}
	rows.Close()
	return db, host, login, senha
}

func novasPassagens(db *sql.DB, ult *int64) []string {
	if *ult < 0 {
		db.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM catraca_passagens`).Scan(ult)
		return nil
	}
	rows, err := db.Query(`SELECT id, COALESCE(name, ''), source FROM catraca_passagens WHERE id > ? AND allowed = 1 ORDER BY id`, *ult)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id int64
		var nome, origem string
		if rows.Scan(&id, &nome, &origem) == nil {
			*ult = id
			out = append(out, fmt.Sprintf("%s [%s]", nome, origem))
		}
	}
	// Negadas também avançam o marcador.
	db.QueryRow(`SELECT COALESCE(MAX(id), ?) FROM catraca_passagens`, *ult).Scan(ult)
	return out
}

// ---------- iDFace (só consultas) ----------

type idface struct {
	host, login, senha, sessao string
}

func (d *idface) post(path string, in, out any) error {
	body, _ := json.Marshal(in)
	url := "http://" + d.host + "/" + path
	if d.sessao != "" {
		url += "?session=" + d.sessao
	}
	res, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sem resposta de %s", d.host)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode == http.StatusUnauthorized && d.sessao != "" {
		if err := d.entrar(); err != nil {
			return err
		}
		return d.post(path, in, out)
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, bytes.TrimSpace(data))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (d *idface) entrar() error {
	d.sessao = ""
	var r struct{ Session string }
	if err := d.post("login.fcgi", map[string]string{"login": d.login, "password": d.senha}, &r); err != nil {
		return err
	}
	if r.Session == "" {
		return fmt.Errorf("usuário ou senha incorretos")
	}
	d.sessao = r.Session
	return nil
}

func (d *idface) logs(ult *int64) ([]map[string]any, error) {
	// Na primeira leitura pega tudo só para achar o último id (o histórico não é mostrado).
	where := map[string]any{"id": map[string]any{">": max(*ult, 0)}}
	var r struct {
		AccessLogs []map[string]any `json:"access_logs"`
	}
	if err := d.post("load_objects.fcgi", map[string]any{"object": "access_logs", "where": map[string]any{"access_logs": where}}, &r); err != nil {
		return nil, err
	}
	primeira := *ult < 0
	if primeira {
		*ult = 0
	}
	var novos []map[string]any
	for _, row := range r.AccessLogs {
		if id, ok := row["id"].(float64); ok && int64(id) > *ult {
			*ult = int64(id)
		}
		if !primeira {
			novos = append(novos, row)
		}
	}
	if primeira {
		say("Histórico antigo do iDFace ignorado (%d registros). Mostrando só o que acontecer agora.", len(r.AccessLogs))
	}
	return novos, nil
}

func (d *idface) portas() (string, error) {
	var r map[string]any
	if err := d.post("door_state.fcgi", map[string]any{}, &r); err != nil {
		return "", err
	}
	b, _ := json.Marshal(r)
	return string(b), nil
}

func compacto(m map[string]any) string {
	b, _ := json.Marshal(m)
	return strings.TrimSpace(string(b))
}
