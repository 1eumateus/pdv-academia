// Caminho: catraca/internal/service/catraca_service.go

package service

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"pdv/internal/actor"
	"pdv/internal/device"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// Permissões da catraca. PermGate é a antiga, que cobria tudo: só aparece em
// cadastros gravados antes da divisão (ver legacyPerms).
const (
	PermGate       = "catraca"
	PermGateMember = "catraca_aluno"          // bloquear, cortesia, observação
	PermGateManual = "catraca_liberar_manual" // abrir sem aluno, com motivo
	PermGateFace   = "catraca_remover_rosto"
	PermGateRemove = "catraca_apagar_aluno"
	PermGateStaff  = "catraca_funcionarios"
	PermGateConfig = "catraca_configurar" // configurar e importar do leitor
)

// CatracaService liga o leitor facial e a catraca às assinaturas: o aluno é o
// cliente do PDV e passa enquanto a mensalidade estiver em dia. Atende o modo
// online do iDFace (servidor HTTP) e aciona a catraca.
type CatracaService struct {
	Base
	repo      repository.CatracaRepo
	settings  repository.SettingsRepo
	customers *CustomerService
	// OnEvent é chamado a cada passagem registrada (a tela atualiza ao vivo).
	OnEvent func(models.GateEvent)

	mu       sync.Mutex
	cfg      models.GateConfig
	dev      *device.ControlID
	srv      *device.Server
	srvErr   string
	lastLog  map[int64]time.Time       // aluno -> último registro do leitor
	lastDec  map[int64]device.Decision // aluno -> resposta dada nesse registro
	enrollMu sync.Mutex

	photoMu sync.Mutex
	photos  map[int64]string // usuário do leitor -> foto (data URL; "" = sem foto)
}

func NewCatracaService(b Base) *CatracaService {
	return &CatracaService{Base: b, customers: NewCustomerService(b), lastLog: map[int64]time.Time{}, lastDec: map[int64]device.Decision{}, photos: map[int64]string{}}
}

// Start lê a configuração e abre o servidor do modo online.
func (s *CatracaService) Start(ctx context.Context) error {
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return err
	}
	s.apply(cfg)
	return nil
}

func (s *CatracaService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		s.srv.Stop()
		s.srv = nil
	}
}

// apply troca a configuração em uso e reabre o servidor se a porta mudou.
func (s *CatracaService) apply(cfg models.GateConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.cfg
	s.cfg = cfg
	s.dev = device.NewControlID(hostPort(cfg.DeviceHost), cfg.DeviceLogin, cfg.DevicePassword, cfg.SecBoxID)
	if s.srv != nil && old.ServerPort == cfg.ServerPort {
		return
	}
	if s.srv != nil {
		s.srv.Stop()
		s.srv = nil
	}
	srv, err := device.Listen(int(cfg.ServerPort), s.identified, s.allowActions)
	if err != nil {
		s.srvErr = fmt.Sprintf("Não foi possível abrir a porta %d: %v", cfg.ServerPort, err)
		return
	}
	s.srv, s.srvErr = srv, ""
}

func hostPort(h string) string {
	h = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(h), "http://"), "https://"))
	return strings.TrimSuffix(h, "/")
}

func (s *CatracaService) current() (models.GateConfig, *device.ControlID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg, s.dev
}

// ---------------- configuração ----------------

func defaultGateConfig() models.GateConfig {
	return models.GateConfig{Relay: models.GateRelayIDFace, ServerPort: 8010, DeviceLogin: "admin", DevicePassword: "admin",
		SecBoxID: 65793, SerialPort: "COM3", SerialSignal: "DTR", PulseMs: 1000}
}

func (s *CatracaService) loadConfig(ctx context.Context) (models.GateConfig, error) {
	m, err := s.settings.All(ctx, s.DB)
	if err != nil {
		return models.GateConfig{}, err
	}
	c := defaultGateConfig()
	str := map[string]*string{"catraca_relay": &c.Relay, "catraca_device_host": &c.DeviceHost, "catraca_device_login": &c.DeviceLogin,
		"catraca_device_password": &c.DevicePassword, "catraca_serial_port": &c.SerialPort, "catraca_serial_signal": &c.SerialSignal}
	num := map[string]*int64{"catraca_server_port": &c.ServerPort, "catraca_tolerance_days": &c.ToleranceDays,
		"catraca_secbox_id": &c.SecBoxID, "catraca_pulse_ms": &c.PulseMs}
	for k, p := range str {
		if v, ok := m[k]; ok {
			*p = v
		}
	}
	for k, p := range num {
		if v, err := strconv.ParseInt(m[k], 10, 64); err == nil {
			*p = v
		}
	}
	return c, nil
}

func validateGateConfig(c models.GateConfig) (models.GateConfig, error) {
	c.DeviceHost, c.SerialPort = hostPort(c.DeviceHost), strings.ToUpper(strings.TrimSpace(c.SerialPort))
	c.SerialSignal = strings.ToUpper(c.SerialSignal)
	switch {
	case c.Relay != models.GateRelayIDFace && c.Relay != models.GateRelaySerial && c.Relay != models.GateRelayNone:
		return c, invalid("Escolha quem aciona a catraca.")
	case c.ServerPort < 1 || c.ServerPort > 65535:
		return c, invalid("Porta do servidor inválida.")
	case c.ToleranceDays < 0 || c.ToleranceDays > 60:
		return c, invalid("A tolerância deve ficar entre 0 e 60 dias.")
	case c.SecBoxID < 0:
		return c, invalid("ID do SecBox inválido.")
	case c.Relay == models.GateRelayIDFace && c.DeviceHost == "":
		return c, invalid("Informe o IP do iDFace.")
	case c.Relay == models.GateRelaySerial && !strings.HasPrefix(c.SerialPort, "COM"):
		return c, invalid("Informe a porta serial (ex.: COM3).")
	case c.SerialSignal != "DTR" && c.SerialSignal != "RTS":
		return c, invalid("O sinal do relé deve ser DTR ou RTS.")
	case c.PulseMs < 100 || c.PulseMs > 10000:
		return c, invalid("O tempo do pulso deve ficar entre 100 e 10000 ms.")
	}
	return c, nil
}

// SaveConfig grava a configuração e já passa a usá-la. Exige a permissão da catraca.
func (s *CatracaService) SaveConfig(ctx context.Context, in models.GateConfig) (models.GateConfig, error) {
	c, err := validateGateConfig(in)
	if err != nil {
		return c, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermGateConfig)
		if err != nil {
			return err
		}
		for k, v := range map[string]string{
			"catraca_relay": c.Relay, "catraca_server_port": strconv.FormatInt(c.ServerPort, 10),
			"catraca_tolerance_days": strconv.FormatInt(c.ToleranceDays, 10), "catraca_device_host": c.DeviceHost,
			"catraca_device_login": c.DeviceLogin, "catraca_device_password": c.DevicePassword,
			"catraca_secbox_id": strconv.FormatInt(c.SecBoxID, 10), "catraca_serial_port": c.SerialPort,
			"catraca_serial_signal": c.SerialSignal, "catraca_pulse_ms": strconv.FormatInt(c.PulseMs, 10)} {
			if err := s.settings.Set(ctx, tx, k, v); err != nil {
				return err
			}
		}
		return audit(ctx, tx, now, authBy, "catraca_configurada", "catraca", nil, nil,
			map[string]any{"acionamento": c.Relay, "idface": c.DeviceHost, "porta": c.ServerPort, "tolerancia": c.ToleranceDays})
	})
	if err != nil {
		return c, err
	}
	s.apply(c)
	return c, nil
}

func (s *CatracaService) Status(ctx context.Context) (models.GateStatus, error) {
	s.mu.Lock()
	st := models.GateStatus{Config: s.cfg, ServerRunning: s.srv != nil, ServerError: s.srvErr, LocalIPs: device.LocalIPs()}
	if s.srv != nil {
		if t := s.srv.LastSeen(); !t.IsZero() {
			st.DeviceLastSeen = &t
		}
	}
	s.mu.Unlock()
	st.Config.DevicePassword = "" // a senha não volta para a tela
	st.SerialPorts = device.SerialPorts()
	now := s.Now()
	y, m, d := now.Date()
	var err error
	st.AllowedToday, st.DeniedToday, err = s.repo.TodayCounts(ctx, s.DB, time.Date(y, m, d, 0, 0, 0, 0, now.Location()))
	return st, err
}

// Config devolve a configuração para editar (com a senha, só para quem pode configurar).
func (s *CatracaService) Config(ctx context.Context) (models.GateConfig, error) {
	if _, err := requirePerm(ctx, s.Now(), PermGateConfig); err != nil {
		return models.GateConfig{}, err
	}
	c, _ := s.current()
	return c, nil
}

// TestDevice confere login e comunicação com o iDFace; devolve quantas pessoas ele tem.
func (s *CatracaService) TestDevice(ctx context.Context) (int, error) {
	c, dev := s.current()
	if c.DeviceHost == "" {
		return 0, invalid("Informe e salve o IP do iDFace antes de testar.")
	}
	users, err := dev.Users(ctx)
	if err != nil {
		return 0, deviceErr(err)
	}
	return len(users), nil
}

// deviceErr: falha de comunicação com o aparelho vira mensagem para a tela.
func deviceErr(err error) error {
	return &Error{Code: CodeInvalid, Message: "iDFace: " + err.Error(), Err: err}
}

// ---------------- regra de acesso ----------------

// GateDecide aplica a regra da catraca:
//   - cliente inativo ou bloqueado na catraca não passa;
//   - funcionário passa;
//   - cortesia válida (até hoje) passa;
//   - plano ativo e não pausado passa enquanto a mensalidade em aberto não
//     venceu há mais que a tolerância. Plano que nunca teve mensalidade paga só
//     passa se o vencimento ainda não chegou (a primeira mensalidade se paga
//     antes de entrar);
//   - e, se o plano tem regras de acesso, só nos dias permitidos e dentro das
//     vezes por semana e das entradas por dia (ver ruleBlock);
//   - sem plano ativo não passa.
func GateDecide(m models.GateMember, now time.Time, toleranceDays int64) (bool, string) {
	today := now.Format(dateLayout)
	if !m.Active {
		return false, "Cadastro de cliente inativo"
	}
	if m.Blocked {
		return false, "Acesso bloqueado"
	}
	if m.Staff {
		if m.Role != "" {
			return true, "Funcionário · " + m.Role
		}
		return true, "Funcionário"
	}
	if m.FreeUntil != "" && m.FreeUntil >= today {
		return true, "Cortesia até " + brDate(m.FreeUntil)
	}
	limit := now.AddDate(0, 0, -int(toleranceDays)).Format(dateLayout)
	var best, overdue *models.GatePlan
	paused, unpaid, ruleWhy := false, false, ""
	for i := range m.Plans {
		p := &m.Plans[i]
		switch {
		case p.Paused:
			paused = true
		case p.DueDate == "":
			// ativa sem mensalidade em aberto: não deveria acontecer; não libera
		case p.DueDate > today || (p.EverPaid && p.DueDate >= limit):
			if why := ruleBlock(p.Rules, m, now); why != "" {
				ruleWhy = cmpStr(ruleWhy, why)
				continue
			}
			if best == nil || p.DueDate > best.DueDate {
				best = p
			}
		case !p.EverPaid:
			unpaid = true
		default:
			if overdue == nil || p.DueDate > overdue.DueDate {
				overdue = p
			}
		}
	}
	switch {
	case best != nil && best.DueDate < today:
		return true, fmt.Sprintf("%s: mensalidade vencida em %s (dentro da tolerância)", best.Name, brDate(best.DueDate))
	case best != nil && best.DueDate == models.PermanentExemptDue:
		return true, best.Name + ": isento"
	case best != nil:
		return true, fmt.Sprintf("%s em dia: próxima mensalidade em %s", best.Name, brDate(best.DueDate))
	case ruleWhy != "":
		return false, ruleWhy
	case overdue != nil:
		return false, fmt.Sprintf("Mensalidade do %s vencida em %s", overdue.Name, brDate(overdue.DueDate))
	case unpaid:
		return false, "Primeira mensalidade ainda não paga"
	case paused:
		return false, "Assinatura pausada"
	}
	return false, "Sem plano ativo"
}

var weekdayShort = []string{"", "seg", "ter", "qua", "qui", "sex", "sáb", "dom"}

// ruleBlock: por que as regras de acesso do plano barram agora ("" = libera).
// Quem já entrou hoje não gasta outra vez da semana.
func ruleBlock(r models.AccessRules, m models.GateMember, now time.Time) string {
	if len(r.Weekdays) > 0 {
		if !slices.Contains(r.Weekdays, int64(now.Weekday()+6)%7+1) {
			names := make([]string, len(r.Weekdays))
			for i, d := range r.Weekdays {
				names[i] = weekdayShort[d]
			}
			return "Hoje não é dia do seu plano (" + strings.Join(names, ", ") + ")"
		}
	}
	if r.DailyLimit > 0 && m.EntriesToday >= r.DailyLimit {
		return fmt.Sprintf("Limite de %d entrada(s) por dia atingido", r.DailyLimit)
	}
	if r.WeeklyLimit > 0 && m.EntriesToday == 0 && m.DaysThisWeek >= r.WeeklyLimit {
		return fmt.Sprintf("Limite de %dx na semana atingido", r.WeeklyLimit)
	}
	return ""
}

// weekUsage preenche DaysThisWeek e EntriesToday (semana de segunda a domingo,
// dia local) com as passagens liberadas. customerID 0 = busca de todos.
func (s *CatracaService) weekUsage(ctx context.Context, customerID int64, ms ...*models.GateMember) error {
	now := s.Now().In(Location())
	y, mo, d := now.Date()
	today := time.Date(y, mo, d, 0, 0, 0, 0, now.Location())
	all, err := s.repo.AllowedSince(ctx, s.DB, customerID, today.AddDate(0, 0, -(int(now.Weekday())+6)%7))
	if err != nil {
		return err
	}
	for _, m := range ms {
		days := map[string]bool{}
		m.EntriesToday = 0
		for _, t := range all[m.CustomerID] {
			t = t.In(now.Location())
			days[t.Format(dateLayout)] = true
			if !t.Before(today) {
				m.EntriesToday++
			}
		}
		m.DaysThisWeek = int64(len(days))
	}
	return nil
}

func firstName(n string) string {
	if f := strings.Fields(n); len(f) > 0 {
		return f[0]
	}
	return n
}

// identified atende o iDFace: o rosto foi reconhecido, libera ou nega.
func (s *CatracaService) identified(ctx context.Context, deviceUserID int64) device.Decision {
	c, _ := s.current()
	now := s.Now()
	m, err := s.repo.MemberByDevice(ctx, s.DB, deviceUserID)
	if err != nil {
		reason := "Rosto sem cadastro neste sistema"
		if !isNotFound(err) {
			reason = "Erro ao consultar o cadastro"
		}
		s.log(ctx, models.GateEvent{At: now, Allowed: false, Reason: reason, Source: "leitor",
			Name: fmt.Sprintf("Usuário %d do leitor", deviceUserID)})
		return device.Decision{Message: reason}
	}
	// O leitor repete a identificação enquanto a pessoa está na frente dele:
	// registra uma vez a cada 10 segundos e repete a mesma resposta (a entrada
	// recém-registrada não pode contar contra ela mesma no limite do dia).
	s.mu.Lock()
	if now.Sub(s.lastLog[m.CustomerID]) < 10*time.Second {
		d := s.lastDec[m.CustomerID]
		s.mu.Unlock()
		return d
	}
	s.mu.Unlock()
	if err := s.weekUsage(ctx, m.CustomerID, &m); err != nil {
		return device.Decision{Message: "Erro ao consultar o cadastro"}
	}
	ok, reason := GateDecide(m, now, c.ToleranceDays)
	msg := reason
	if ok {
		msg = "Bem-vindo(a), " + firstName(m.Name) + "!"
	}
	d := device.Decision{Allowed: ok, Name: m.Name, Message: msg}
	s.mu.Lock()
	s.lastLog[m.CustomerID], s.lastDec[m.CustomerID] = now, d
	s.mu.Unlock()
	s.log(ctx, models.GateEvent{At: now, CustomerID: &m.CustomerID, Name: m.Name, Allowed: ok, Reason: reason, Source: "leitor"})
	if ok && c.Relay == models.GateRelaySerial {
		go s.pulse(c)
	}
	return d
}

// allowActions: o iDFace abre a catraca na própria resposta quando ele é o acionador.
func (s *CatracaService) allowActions() []map[string]string {
	c, dev := s.current()
	if c.Relay != models.GateRelayIDFace {
		return []map[string]string{}
	}
	return []map[string]string{dev.ReleaseAction(1)}
}

func (s *CatracaService) pulse(c models.GateConfig) error {
	return device.Pulse(c.SerialPort, c.SerialSignal, time.Duration(c.PulseMs)*time.Millisecond)
}

// release abre a catraca agora (balcão ou manual).
func (s *CatracaService) release(ctx context.Context) error {
	c, dev := s.current()
	var err error
	switch c.Relay {
	case models.GateRelayIDFace:
		err = dev.Release(ctx)
	case models.GateRelaySerial:
		err = s.pulse(c)
	}
	if err != nil {
		return deviceErr(err)
	}
	return nil
}

func (s *CatracaService) log(ctx context.Context, e models.GateEvent) {
	id, err := s.repo.AddEvent(ctx, s.DB, e, actor.ID(ctx))
	if err != nil {
		return
	}
	e.ID = id
	if u, ok := actor.From(ctx); ok {
		e.UserName = u.Name
	}
	if s.OnEvent != nil {
		s.OnEvent(e)
	}
}

// CheckAndRelease: o operador escolhe o aluno no balcão; se pode, abre a catraca.
func (s *CatracaService) CheckAndRelease(ctx context.Context, customerID int64) (models.GateCheck, error) {
	c, _ := s.current()
	m, err := s.repo.Member(ctx, s.DB, customerID)
	if err != nil {
		return models.GateCheck{}, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", customerID), "")
	}
	if err := s.weekUsage(ctx, customerID, &m); err != nil {
		return models.GateCheck{}, err
	}
	ok, reason := GateDecide(m, s.Now(), c.ToleranceDays)
	res := models.GateCheck{Allowed: ok, Reason: reason}
	if ok {
		if err := s.release(ctx); err != nil {
			return res, err
		}
		res.Released = c.Relay != models.GateRelayNone
	}
	s.log(ctx, models.GateEvent{At: s.Now(), CustomerID: &m.CustomerID, Name: m.Name, Allowed: ok, Reason: reason, Source: "balcao"})
	return res, nil
}

// manualNoReason: o que fica registrado quando a liberação manual vem sem motivo.
const manualNoReason = "Liberação manual"

// ManualRelease abre a catraca sem aluno (visitante, aula experimental, teste).
// O motivo é opcional.
func (s *CatracaService) ManualRelease(ctx context.Context, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = manualNoReason
	}
	if len([]rune(reason)) > 200 {
		return invalid("O motivo deve ter no máximo 200 caracteres.")
	}
	if _, err := requirePerm(ctx, s.Now(), PermGateManual); err != nil {
		return err
	}
	if err := s.release(ctx); err != nil {
		return err
	}
	s.log(ctx, models.GateEvent{At: s.Now(), Allowed: true, Reason: reason, Source: "manual"})
	return nil
}

func (s *CatracaService) Events(ctx context.Context, limit int) ([]models.GateEvent, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	return s.repo.Events(ctx, s.DB, limit)
}

// EventReport: passagens de um período (datas locais AAAA-MM-DD, inclusivas).
// status: "" todas, "liberadas" ou "negadas". Qualquer usuário logado vê.
func (s *CatracaService) EventReport(ctx context.Context, start, end, term, status string) ([]models.GateEvent, error) {
	from, to, err := PeriodRange(start, end)
	if err != nil {
		return nil, err
	}
	var allowed *bool
	switch status {
	case "":
	case "liberadas", "negadas":
		v := status == "liberadas"
		allowed = &v
	default:
		return nil, invalid("Situação inválida.")
	}
	return s.repo.EventsBetween(ctx, s.DB, from, to, strings.TrimSpace(term), allowed)
}

// ---------------- alunos e rostos ----------------

func (s *CatracaService) withDecision(m models.GateMember) models.GateMember {
	c, _ := s.current()
	m.Allowed, m.Reason = GateDecide(m, s.Now(), c.ToleranceDays)
	return m
}

// Members lista os alunos com a situação na catraca (ver CatracaRepo.Members).
func (s *CatracaService) Members(ctx context.Context, term string) ([]models.GateMember, error) {
	list, err := s.repo.Members(ctx, s.DB, term)
	if err != nil {
		return list, err
	}
	ms := make([]*models.GateMember, len(list))
	for i := range list {
		ms[i] = &list[i]
	}
	if err := s.weekUsage(ctx, 0, ms...); err != nil {
		return list, err
	}
	for i := range list {
		list[i] = s.withDecision(list[i])
	}
	return list, nil
}

func (s *CatracaService) member(ctx context.Context, customerID int64) (models.GateMember, error) {
	m, err := s.repo.Member(ctx, s.DB, customerID)
	if err != nil {
		return m, fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", customerID), "")
	}
	if err := s.weekUsage(ctx, customerID, &m); err != nil {
		return m, err
	}
	return s.withDecision(m), nil
}

// SaveMember grava bloqueio, cortesia e observação do aluno. Exige a permissão da catraca.
func (s *CatracaService) SaveMember(ctx context.Context, in models.GateMemberInput) (models.GateMember, error) {
	free := strings.TrimSpace(in.FreeUntil)
	notes := strings.TrimSpace(in.Notes)
	if free != "" {
		if _, err := time.Parse(dateLayout, free); err != nil {
			return models.GateMember{}, invalid("Data da cortesia inválida.")
		}
	}
	if len([]rune(notes)) > 500 {
		return models.GateMember{}, invalid("A observação deve ter no máximo 500 caracteres.")
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermGateMember)
		if err != nil {
			return err
		}
		before, err := s.repo.Member(ctx, tx, in.CustomerID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", in.CustomerID), "")
		}
		var freePtr *string
		if free != "" {
			freePtr = &free
		}
		if err := s.repo.SaveMember(ctx, tx, in.CustomerID, in.Blocked, freePtr, notes, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "catraca_aluno_alterado", "cliente", &in.CustomerID,
			map[string]any{"bloqueado": before.Blocked, "cortesia_ate": before.FreeUntil},
			map[string]any{"bloqueado": in.Blocked, "cortesia_ate": free})
	})
	if err != nil {
		return models.GateMember{}, err
	}
	return s.member(ctx, in.CustomerID)
}

// ---------------- funcionários ----------------

// Staff lista os funcionários com a situação na catraca.
func (s *CatracaService) Staff(ctx context.Context) ([]models.GateMember, error) {
	list, err := s.repo.Staff(ctx, s.DB)
	for i := range list {
		list[i] = s.withDecision(list[i])
	}
	return list, err
}

// SaveStaff cadastra um funcionário novo (CustomerID 0) ou faz de um cliente
// existente funcionário (ativando o cadastro), ou troca a função. Exige a permissão da catraca.
func (s *CatracaService) SaveStaff(ctx context.Context, in models.GateStaffInput) (models.GateMember, error) {
	role := strings.TrimSpace(in.Role)
	if len([]rune(role)) > 60 {
		return models.GateMember{}, invalid("A função deve ter no máximo 60 caracteres.")
	}
	id := in.CustomerID
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermGateStaff)
		if err != nil {
			return err
		}
		var custs repository.CustomerRepo
		if id == 0 {
			ci := models.CustomerInput{Name: in.Name, Active: true}
			if p := strings.TrimSpace(in.Phone); p != "" {
				ci.Phone = &p
			}
			f, err := validateCustomer(ci)
			if err != nil {
				return err
			}
			if id, err = custs.Insert(ctx, tx, f, now); err != nil {
				return fromRepo(err, nil, "")
			}
		} else {
			c, err := custs.GetByID(ctx, tx, id)
			if err != nil {
				return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", id), "")
			}
			if !c.Active {
				if err := custs.SetActive(ctx, tx, id, true, now); err != nil {
					return err
				}
			}
		}
		if err := s.repo.SetStaff(ctx, tx, id, &role, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "catraca_funcionario", "cliente", &id, nil, map[string]any{"funcao": role})
	})
	if err != nil {
		return models.GateMember{}, err
	}
	return s.member(ctx, id)
}

// RemoveStaff desliga o funcionário: volta a ser cliente comum (sem plano, a
// catraca nega). O rosto continua no leitor. Exige a permissão da catraca.
func (s *CatracaService) RemoveStaff(ctx context.Context, customerID int64) (models.GateMember, error) {
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermGateStaff)
		if err != nil {
			return err
		}
		if err := s.repo.SetStaff(ctx, tx, customerID, nil, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "catraca_funcionario_desligado", "cliente", &customerID, nil, nil)
	})
	if err != nil {
		return models.GateMember{}, err
	}
	return s.member(ctx, customerID)
}

// enrollTarget prepara o cadastro do rosto: confere o aparelho e o aluno e
// cria a pessoa no iDFace se ela ainda não existe lá.
func (s *CatracaService) enrollTarget(ctx context.Context, customerID int64) (models.GateMember, *device.ControlID, error) {
	c, dev := s.current()
	if c.DeviceHost == "" {
		return models.GateMember{}, nil, invalid("Configure o IP do iDFace antes de cadastrar rostos.")
	}
	m, err := s.member(ctx, customerID)
	if err != nil {
		return m, nil, err
	}
	if !m.Active {
		return m, nil, newErr(CodeCustomerInactive, "O cliente %s está inativo.", m.Name)
	}
	if m.DeviceUserID == nil {
		// A matrícula no aparelho é C + código do cliente: a importação liga por ela.
		id, err := dev.CreateUser(ctx, m.Name, "C"+strconv.FormatInt(m.CustomerID, 10))
		if err != nil {
			return m, nil, deviceErr(err)
		}
		if err := s.repo.SetDevice(ctx, s.DB, m.CustomerID, &id, s.Now()); err != nil {
			return m, nil, fromRepo(err, nil, "Este usuário do leitor já está ligado a outro cliente.")
		}
		m.DeviceUserID = &id
	}
	return m, dev, nil
}

// maxFacePhoto: tamanho máximo da foto enviada para cadastrar o rosto.
const maxFacePhoto = 4 << 20

// EnrollFacePhoto cadastra o rosto do aluno a partir de uma foto (JPEG ou PNG),
// sem precisar ficar na frente do leitor.
func (s *CatracaService) EnrollFacePhoto(ctx context.Context, customerID int64, img []byte) (models.GateMember, error) {
	if len(img) == 0 {
		return models.GateMember{}, invalid("Escolha uma foto.")
	}
	if len(img) > maxFacePhoto {
		return models.GateMember{}, invalid("A foto é grande demais (máximo %d MB).", maxFacePhoto>>20)
	}
	if ct := http.DetectContentType(img); ct != "image/jpeg" && ct != "image/png" {
		return models.GateMember{}, invalid("Use uma foto JPG ou PNG.")
	}
	if !s.enrollMu.TryLock() {
		return models.GateMember{}, newErr(CodeStateConflict, "Já há um cadastro de rosto em andamento no leitor.")
	}
	defer s.enrollMu.Unlock()
	m, dev, err := s.enrollTarget(ctx, customerID)
	if err != nil {
		return m, err
	}
	if err := dev.SetUserImage(ctx, *m.DeviceUserID, img); err != nil {
		return m, deviceErr(err)
	}
	s.forgetPhoto(*m.DeviceUserID)
	return s.member(ctx, m.CustomerID)
}

// EnrollFace cria o aluno no iDFace (se ainda não existe lá) e abre a câmera
// do aparelho para cadastrar o rosto. Só um cadastro por vez.
func (s *CatracaService) EnrollFace(ctx context.Context, customerID int64) (models.GateMember, error) {
	if !s.enrollMu.TryLock() {
		return models.GateMember{}, newErr(CodeStateConflict, "Já há um cadastro de rosto em andamento no leitor.")
	}
	defer s.enrollMu.Unlock()
	m, dev, err := s.enrollTarget(ctx, customerID)
	if err != nil {
		return m, err
	}
	if err := dev.EnrollFace(ctx, *m.DeviceUserID); err != nil {
		return m, deviceErr(err)
	}
	s.forgetPhoto(*m.DeviceUserID)
	return s.member(ctx, m.CustomerID)
}

// RemoveFace apaga o aluno do leitor e desfaz o vínculo. Exige a permissão da catraca.
func (s *CatracaService) RemoveFace(ctx context.Context, customerID int64) (models.GateMember, error) {
	if _, err := requirePerm(ctx, s.Now(), PermGateFace); err != nil {
		return models.GateMember{}, err
	}
	m, err := s.member(ctx, customerID)
	if err != nil || m.DeviceUserID == nil {
		return m, err
	}
	if c, dev := s.current(); c.DeviceHost != "" {
		if err := dev.DeleteUser(ctx, *m.DeviceUserID); err != nil {
			return m, deviceErr(fmt.Errorf("não foi possível apagar a pessoa do aparelho: %w", err))
		}
	}
	s.forgetPhoto(*m.DeviceUserID)
	if err := s.repo.SetDevice(ctx, s.DB, customerID, nil, s.Now()); err != nil {
		return m, err
	}
	return s.member(ctx, customerID)
}

// RemoveMember tira o aluno da academia: apaga o rosto do leitor, cancela as
// assinaturas ativas e desativa o cliente. Vendas, mensalidades e passagens
// ficam guardadas; "Reativar" (Assinaturas) traz de volta. Exige a permissão da catraca.
func (s *CatracaService) RemoveMember(ctx context.Context, customerID int64) error {
	if _, err := requirePerm(ctx, s.Now(), PermGateRemove); err != nil {
		return err
	}
	m, err := s.member(ctx, customerID)
	if err != nil {
		return err
	}
	// O aparelho primeiro: se ele falhar, nada muda aqui.
	if c, dev := s.current(); m.DeviceUserID != nil && c.DeviceHost != "" {
		if err := dev.DeleteUser(ctx, *m.DeviceUserID); err != nil {
			return deviceErr(fmt.Errorf("não foi possível apagar a pessoa do aparelho: %w", err))
		}
		s.forgetPhoto(*m.DeviceUserID)
	}
	return s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermGateRemove)
		if err != nil {
			return err
		}
		var subs repository.SubscriptionRepo
		ids, err := subs.ActiveIDs(ctx, tx, customerID)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if _, err := subs.Cancel(ctx, tx, id, now); err != nil {
				return err
			}
		}
		if err := (repository.CustomerRepo{}).SetActive(ctx, tx, customerID, false, now); err != nil {
			return err
		}
		if err := s.repo.DeleteMember(ctx, tx, customerID); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "catraca_aluno_apagado", "cliente", &customerID, nil,
			map[string]any{"nome": m.Name, "assinaturas_canceladas": len(ids)})
	})
}

// DeleteCustomer exclui de vez um cliente sem histórico (cadastro por engano).
// Com vendas, fiado, assinaturas ou passagens, recusa: use inativar. Se ele tem
// rosto no leitor, apaga de lá antes de confirmar a exclusão.
func (s *CatracaService) DeleteCustomer(ctx context.Context, customerID int64) error {
	var device *int64
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		m, err := s.repo.Member(ctx, tx, customerID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", customerID), "")
		}
		if err := s.repo.DeleteMember(ctx, tx, customerID); err != nil {
			return err
		}
		if err := (repository.CustomerRepo{}).Delete(ctx, tx, customerID); err != nil {
			if errors.Is(err, repository.ErrForeignKey) {
				return newErr(CodeStateConflict, "%s tem histórico (vendas, fiado, assinaturas ou passagens) e não pode ser excluído. Use Inativar.", m.Name)
			}
			return fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", customerID), "")
		}
		// O aparelho por último, ainda dentro da transação: se ele falhar, nada é excluído.
		if c, dev := s.current(); m.DeviceUserID != nil && c.DeviceHost != "" {
			if err := dev.DeleteUser(ctx, *m.DeviceUserID); err != nil {
				return deviceErr(fmt.Errorf("não foi possível apagar a pessoa do aparelho: %w", err))
			}
		}
		device = m.DeviceUserID
		return audit(ctx, tx, s.Now(), nil, "cliente_excluido", "cliente", &customerID, map[string]any{"nome": m.Name}, nil)
	})
	if err == nil && device != nil {
		s.forgetPhoto(*device)
	}
	return err
}

// Photo devolve a foto do aluno guardada no iDFace, pronta para a tela (data
// URL), ou "" se ele não tem rosto cadastrado. Fica em memória até o rosto ser
// cadastrado de novo ou removido.
func (s *CatracaService) Photo(ctx context.Context, customerID int64) (string, error) {
	m, err := s.repo.Member(ctx, s.DB, customerID)
	if err != nil || m.DeviceUserID == nil {
		return "", nil
	}
	id := *m.DeviceUserID
	s.photoMu.Lock()
	url, ok := s.photos[id]
	s.photoMu.Unlock()
	if ok {
		return url, nil
	}
	c, dev := s.current()
	if c.DeviceHost == "" {
		return "", nil
	}
	img, err := dev.UserImage(ctx, id)
	if err != nil {
		return "", deviceErr(err)
	}
	if len(img) > 0 {
		url = "data:" + http.DetectContentType(img) + ";base64," + base64.StdEncoding.EncodeToString(img)
	}
	s.photoMu.Lock()
	s.photos[id] = url
	s.photoMu.Unlock()
	return url, nil
}

func (s *CatracaService) forgetPhoto(deviceUserID int64) {
	s.photoMu.Lock()
	delete(s.photos, deviceUserID)
	s.photoMu.Unlock()
}

// normName: nome para comparar (minúsculas, espaços únicos).
func normName(n string) string { return strings.ToLower(strings.Join(strings.Fields(n), " ")) }

// ImportFromDevice liga ao cadastro de clientes as pessoas que já estão no
// iDFace (ex.: cadastradas por outro sistema). Liga pela matrícula "C<código
// do cliente>" e, se não houver, pelo nome igual (só quando um único cliente
// tem esse nome); quem não casa vira cliente novo. Exige a permissão da catraca.
func (s *CatracaService) ImportFromDevice(ctx context.Context) (models.GateImport, error) {
	if _, err := requirePerm(ctx, s.Now(), PermGateConfig); err != nil {
		return models.GateImport{}, err
	}
	c, dev := s.current()
	if c.DeviceHost == "" {
		return models.GateImport{}, invalid("Configure o IP do iDFace antes de importar.")
	}
	users, err := dev.Users(ctx)
	if err != nil {
		return models.GateImport{}, deviceErr(err)
	}
	cands, err := s.repo.LinkCandidates(ctx, s.DB)
	if err != nil {
		return models.GateImport{}, err
	}
	byDevice, byID, byName := map[int64]bool{}, map[int64]*repository.LinkCandidate{}, map[string][]*repository.LinkCandidate{}
	for i := range cands {
		cd := &cands[i]
		if cd.DeviceID != nil {
			byDevice[*cd.DeviceID] = true
		}
		byID[cd.ID] = cd
		byName[normName(cd.Name)] = append(byName[normName(cd.Name)], cd)
	}
	res := models.GateImport{DeviceUsers: len(users)}
	now := s.Now()
	link := func(cd *repository.LinkCandidate, deviceID int64) error {
		if err := s.repo.SetDevice(ctx, s.DB, cd.ID, &deviceID, now); err != nil {
			return err
		}
		cd.DeviceID = &deviceID
		byDevice[deviceID] = true
		return nil
	}
	for _, u := range users {
		if byDevice[u.ID] {
			res.Already++
			continue
		}
		reg := strings.TrimSpace(u.Registration)
		if id, err := strconv.ParseInt(strings.TrimPrefix(strings.ToUpper(reg), "C"), 10, 64); err == nil && strings.HasPrefix(strings.ToUpper(reg), "C") {
			if cd := byID[id]; cd != nil && cd.DeviceID == nil {
				if err := link(cd, u.ID); err != nil {
					return res, err
				}
				res.Linked++
				continue
			}
		}
		name := strings.Join(strings.Fields(u.Name), " ")
		if same := byName[normName(name)]; name != "" && len(same) == 1 && same[0].DeviceID == nil {
			if err := link(same[0], u.ID); err != nil {
				return res, err
			}
			res.Linked++
			continue
		}
		if name == "" {
			name = fmt.Sprintf("Usuário %d do leitor", u.ID)
		}
		cust, err := s.customers.Create(ctx, models.CustomerInput{Name: name, Active: true})
		if err != nil {
			return res, err
		}
		cd := &repository.LinkCandidate{ID: cust.ID, Name: cust.Name}
		byID[cd.ID] = cd
		if err := link(cd, u.ID); err != nil {
			return res, err
		}
		res.Created++
	}
	return res, nil
}
