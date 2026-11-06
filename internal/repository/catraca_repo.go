// Caminho: catraca/internal/repository/catraca_repo.go

package repository

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"pdv/internal/models"
)

// CatracaRepo: alunos (clientes) vistos pela catraca e as passagens.
type CatracaRepo struct{}

const gateMemberSelect = `SELECT c.id, c.name, c.phone, c.active, a.device_user_id, COALESCE(a.blocked, 0), COALESCE(a.free_until, ''),
	COALESCE(a.notes, ''), (SELECT MAX(e.at) FROM catraca_passagens e WHERE e.customer_id = c.id AND e.allowed = 1), a.funcao
	FROM customers c LEFT JOIN catraca_alunos a ON a.customer_id = c.id`

func scanGateMember(s interface{ Scan(...any) error }) (models.GateMember, error) {
	var (
		m      models.GateMember
		phone  sql.NullString
		device sql.NullInt64
		last   sql.NullString
		role   sql.NullString
		err    error
	)
	if err = s.Scan(&m.CustomerID, &m.Name, &phone, &m.Active, &device, &m.Blocked, &m.FreeUntil, &m.Notes, &last, &role); err != nil {
		return m, err
	}
	m.Phone, m.DeviceUserID, m.Plans = nullStr(phone), nullInt(device), []models.GatePlan{}
	m.Staff, m.Role = role.Valid, role.String
	m.LastAccess, err = parseNullTime(last)
	return m, err
}

// gatePlans traz as assinaturas ativas, agrupadas por cliente (customerID 0 = todos),
// com as regras de acesso que valem: a exceção da assinatura ou, sem ela, as do plano.
// EverPaid: tem mensalidade paga ou veio do sistema antigo (importada): quem já era
// aluno lá não é tratado como assinatura nova que paga a primeira antes de entrar.
func gatePlans(ctx context.Context, q DBTX, customerID int64) (map[int64][]models.GatePlan, error) {
	const op = "ler planos do aluno"
	rows, err := q.QueryContext(ctx, `SELECT s.customer_id, p.name, COALESCE(i.due_date, ''), s.paused_at IS NOT NULL,
		EXISTS (SELECT 1 FROM subscription_invoices x WHERE x.subscription_id = s.id AND (x.status = 'paga' OR x.imported = 1)),
		COALESCE(r.dias, pr.dias), COALESCE(r.vezes_semana, pr.vezes_semana), COALESCE(r.entradas_dia, pr.entradas_dia)
		FROM subscriptions s JOIN products p ON p.id = s.product_id
		LEFT JOIN subscription_invoices i ON i.subscription_id = s.id AND i.status = 'aberta'
		LEFT JOIN catraca_regras r ON r.subscription_id = s.id
		LEFT JOIN catraca_plano_regras pr ON pr.product_id = s.product_id
		WHERE s.status = 'ativa' AND (? = 0 OR s.customer_id = ?)
		ORDER BY s.id`, customerID, customerID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := map[int64][]models.GatePlan{}
	for rows.Next() {
		var (
			id            int64
			p             models.GatePlan
			days          sql.NullString
			weekly, daily sql.NullInt64
		)
		if err := rows.Scan(&id, &p.Name, &p.DueDate, &p.Paused, &p.EverPaid, &days, &weekly, &daily); err != nil {
			return nil, wrap(op, err)
		}
		p.Rules = rulesFrom(days, weekly, daily)
		out[id] = append(out[id], p)
	}
	return out, wrap(op, rows.Err())
}

func (CatracaRepo) memberWhere(ctx context.Context, q DBTX, where string, args ...any) (models.GateMember, error) {
	const op = "ler aluno"
	m, err := scanGateMember(q.QueryRowContext(ctx, gateMemberSelect+" WHERE "+where, args...))
	if err != nil {
		return m, wrap(op, err)
	}
	plans, err := gatePlans(ctx, q, m.CustomerID)
	if err != nil {
		return m, err
	}
	if p := plans[m.CustomerID]; p != nil {
		m.Plans = p
	}
	return m, nil
}

// Member devolve o cliente visto pela catraca (ErrNotFound se não existe).
func (r CatracaRepo) Member(ctx context.Context, q DBTX, customerID int64) (models.GateMember, error) {
	return r.memberWhere(ctx, q, "c.id = ?", customerID)
}

// MemberByDevice devolve quem tem o usuário do leitor (ErrNotFound se ninguém).
func (r CatracaRepo) MemberByDevice(ctx context.Context, q DBTX, deviceUserID int64) (models.GateMember, error) {
	return r.memberWhere(ctx, q, "a.device_user_id = ?", deviceUserID)
}

// Members lista por nome. Sem busca: quem já teve assinatura (ativa ou não:
// os ex-alunos aparecem como inativos, menos quem foi apagado: cliente
// desativado) ou tem cadastro na catraca, menos os
// funcionários. Com busca: qualquer cliente ativo (para cadastrar o rosto).
func (r CatracaRepo) Members(ctx context.Context, q DBTX, term string) ([]models.GateMember, error) {
	term = strings.TrimSpace(term)
	where := ` WHERE a.funcao IS NULL AND (a.customer_id IS NOT NULL OR
		(EXISTS (SELECT 1 FROM subscriptions s WHERE s.customer_id = c.id AND s.status = 'ativa')) OR
		(c.active = 1 AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.customer_id = c.id)))`
	var args []any
	if term != "" {
		like := "%" + escapeLike(term) + "%"
		where = ` WHERE c.active = 1 AND (c.name LIKE ? ESCAPE '\' OR IFNULL(c.phone, '') LIKE ? ESCAPE '\' OR IFNULL(c.document, '') LIKE ? ESCAPE '\')`
		args = []any{like, like, like}
	}
	return r.list(ctx, q, where, args...)
}

// Staff lista os funcionários (ativos e desligados do cadastro de clientes).
func (r CatracaRepo) Staff(ctx context.Context, q DBTX) ([]models.GateMember, error) {
	return r.list(ctx, q, ` WHERE a.funcao IS NOT NULL`)
}

func (CatracaRepo) list(ctx context.Context, q DBTX, where string, args ...any) ([]models.GateMember, error) {
	const op = "listar alunos"
	rows, err := q.QueryContext(ctx, gateMemberSelect+where+` ORDER BY c.name COLLATE NOCASE LIMIT 2000`, args...)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.GateMember{}
	for rows.Next() {
		m, err := scanGateMember(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, wrap(op, err)
	}
	rows.Close()
	plans, err := gatePlans(ctx, q, 0)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if p := plans[out[i].CustomerID]; p != nil {
			out[i].Plans = p
		}
	}
	return out, nil
}

// SaveMember grava bloqueio, cortesia e observação (cria a linha se preciso).
func (CatracaRepo) SaveMember(ctx context.Context, q DBTX, customerID int64, blocked bool, freeUntil *string, notes string, now time.Time) error {
	_, err := q.ExecContext(ctx, `INSERT INTO catraca_alunos (customer_id, blocked, free_until, notes, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (customer_id) DO UPDATE SET blocked = excluded.blocked, free_until = excluded.free_until, notes = excluded.notes,
		updated_at = excluded.updated_at`, customerID, blocked, freeUntil, notes, FormatTime(now))
	return wrap("salvar aluno na catraca", err)
}

// rulesFrom monta as regras de acesso a partir das colunas de catraca_regras.
func rulesFrom(days sql.NullString, weekly, daily sql.NullInt64) models.AccessRules {
	r := models.AccessRules{Weekdays: []int64{}, WeeklyLimit: weekly.Int64, DailyLimit: daily.Int64}
	for _, d := range strings.Split(days.String, ",") {
		if n, err := strconv.ParseInt(d, 10, 64); err == nil {
			r.Weekdays = append(r.Weekdays, n)
		}
	}
	return r
}

func daysText(r models.AccessRules) string {
	days := make([]string, len(r.Weekdays))
	for i, d := range r.Weekdays {
		days[i] = strconv.FormatInt(d, 10)
	}
	return strings.Join(days, ",")
}

// SetRules grava a exceção de regras da assinatura (nil = volta a seguir o plano).
// Exceção sem restrição também é gravada: o aluno fica livre mesmo num plano limitado.
func (CatracaRepo) SetRules(ctx context.Context, q DBTX, subID int64, r *models.AccessRules) error {
	const op = "gravar regras de acesso"
	if r == nil {
		_, err := q.ExecContext(ctx, `DELETE FROM catraca_regras WHERE subscription_id = ?`, subID)
		return wrap(op, err)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO catraca_regras (subscription_id, dias, vezes_semana, entradas_dia) VALUES (?, ?, ?, ?)
		ON CONFLICT (subscription_id) DO UPDATE SET dias = excluded.dias, vezes_semana = excluded.vezes_semana, entradas_dia = excluded.entradas_dia`,
		subID, daysText(*r), r.WeeklyLimit, r.DailyLimit)
	return wrap(op, err)
}

// SetPlanRules grava as regras do plano (sem restrição = apaga a linha).
func (CatracaRepo) SetPlanRules(ctx context.Context, q DBTX, productID int64, r models.AccessRules) error {
	const op = "gravar regras do plano"
	if len(r.Weekdays) == 0 && r.WeeklyLimit == 0 && r.DailyLimit == 0 {
		_, err := q.ExecContext(ctx, `DELETE FROM catraca_plano_regras WHERE product_id = ?`, productID)
		return wrap(op, err)
	}
	_, err := q.ExecContext(ctx, `INSERT INTO catraca_plano_regras (product_id, dias, vezes_semana, entradas_dia) VALUES (?, ?, ?, ?)
		ON CONFLICT (product_id) DO UPDATE SET dias = excluded.dias, vezes_semana = excluded.vezes_semana, entradas_dia = excluded.entradas_dia`,
		productID, daysText(r), r.WeeklyLimit, r.DailyLimit)
	return wrap(op, err)
}

// PlanRules devolve as regras de todos os planos que têm alguma (por produto).
func (CatracaRepo) PlanRules(ctx context.Context, q DBTX) (map[int64]models.AccessRules, error) {
	const op = "ler regras dos planos"
	rows, err := q.QueryContext(ctx, `SELECT product_id, dias, vezes_semana, entradas_dia FROM catraca_plano_regras`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := map[int64]models.AccessRules{}
	for rows.Next() {
		var (
			id            int64
			days          sql.NullString
			weekly, daily sql.NullInt64
		)
		if err := rows.Scan(&id, &days, &weekly, &daily); err != nil {
			return nil, wrap(op, err)
		}
		out[id] = rulesFrom(days, weekly, daily)
	}
	return out, wrap(op, rows.Err())
}

// AllowedSince devolve, por cliente, os horários das passagens liberadas desde from
// (customerID 0 = todos). A contagem por dia fica com quem chama (dia local).
func (CatracaRepo) AllowedSince(ctx context.Context, q DBTX, customerID int64, from time.Time) (map[int64][]time.Time, error) {
	const op = "contar entradas da semana"
	rows, err := q.QueryContext(ctx, `SELECT customer_id, at FROM catraca_passagens
		WHERE allowed = 1 AND customer_id IS NOT NULL AND at >= ? AND (? = 0 OR customer_id = ?)`, FormatTime(from), customerID, customerID)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := map[int64][]time.Time{}
	for rows.Next() {
		var (
			id int64
			at string
		)
		if err := rows.Scan(&id, &at); err != nil {
			return nil, wrap(op, err)
		}
		t, err := parseTime(at)
		if err != nil {
			return nil, wrap(op, err)
		}
		out[id] = append(out[id], t)
	}
	return out, wrap(op, rows.Err())
}

// DeleteMember apaga a situação do aluno na catraca (rosto, bloqueio, cortesia, função).
func (CatracaRepo) DeleteMember(ctx context.Context, q DBTX, customerID int64) error {
	_, err := q.ExecContext(ctx, `DELETE FROM catraca_alunos WHERE customer_id = ?`, customerID)
	return wrap("tirar aluno da catraca", err)
}

// SetStaff marca o cliente como funcionário com a função (nil = volta a ser aluno).
func (CatracaRepo) SetStaff(ctx context.Context, q DBTX, customerID int64, role *string, now time.Time) error {
	_, err := q.ExecContext(ctx, `INSERT INTO catraca_alunos (customer_id, funcao, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (customer_id) DO UPDATE SET funcao = excluded.funcao, updated_at = excluded.updated_at`,
		customerID, role, FormatTime(now))
	return wrap("gravar funcionário", err)
}

// SetDevice liga (ou desliga, com nil) o cliente ao usuário do leitor.
func (CatracaRepo) SetDevice(ctx context.Context, q DBTX, customerID int64, deviceUserID *int64, now time.Time) error {
	_, err := q.ExecContext(ctx, `INSERT INTO catraca_alunos (customer_id, device_user_id, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (customer_id) DO UPDATE SET device_user_id = excluded.device_user_id, updated_at = excluded.updated_at`,
		customerID, deviceUserID, FormatTime(now))
	return wrap("ligar aluno ao leitor", err)
}

// LinkCandidate: cliente para a importação do iDFace.
type LinkCandidate struct {
	ID       int64
	Name     string
	DeviceID *int64
}

// LinkCandidates traz todos os clientes ativos com o vínculo atual no leitor.
func (CatracaRepo) LinkCandidates(ctx context.Context, q DBTX) ([]LinkCandidate, error) {
	const op = "listar clientes para o leitor"
	rows, err := q.QueryContext(ctx, `SELECT c.id, c.name, a.device_user_id FROM customers c
		LEFT JOIN catraca_alunos a ON a.customer_id = c.id WHERE c.active = 1 OR a.device_user_id IS NOT NULL`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []LinkCandidate{}
	for rows.Next() {
		var (
			c   LinkCandidate
			dev sql.NullInt64
		)
		if err := rows.Scan(&c.ID, &c.Name, &dev); err != nil {
			return nil, wrap(op, err)
		}
		c.DeviceID = nullInt(dev)
		out = append(out, c)
	}
	return out, wrap(op, rows.Err())
}

// AddEvent registra uma passagem; userID: operador logado (nil = leitor).
func (CatracaRepo) AddEvent(ctx context.Context, q DBTX, e models.GateEvent, userID any) (int64, error) {
	const op = "registrar passagem"
	res, err := q.ExecContext(ctx, `INSERT INTO catraca_passagens (at, customer_id, name, allowed, reason, source, user_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		FormatTime(e.At), e.CustomerID, e.Name, e.Allowed, e.Reason, e.Source, userID)
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// Events lista as últimas passagens (mais recentes primeiro).
func (CatracaRepo) Events(ctx context.Context, q DBTX, limit int) ([]models.GateEvent, error) {
	const op = "listar passagens"
	rows, err := q.QueryContext(ctx, `SELECT e.id, e.at, e.customer_id, e.name, e.allowed, e.reason, e.source, COALESCE(u.name, '')
		FROM catraca_passagens e LEFT JOIN users u ON u.id = e.user_id ORDER BY e.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.GateEvent{}
	for rows.Next() {
		var (
			e   models.GateEvent
			at  string
			cid sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &at, &cid, &e.Name, &e.Allowed, &e.Reason, &e.Source, &e.UserName); err != nil {
			return nil, wrap(op, err)
		}
		e.CustomerID = nullInt(cid)
		if e.At, err = parseTime(at); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, e)
	}
	return out, wrap(op, rows.Err())
}

// EventsBetween lista as passagens de [from, to) para o relatório (mais antigas
// primeiro). term filtra pelo nome; allowed nil traz liberadas e negadas.
func (CatracaRepo) EventsBetween(ctx context.Context, q DBTX, from, to time.Time, term string, allowed *bool) ([]models.GateEvent, error) {
	const op = "listar passagens do período"
	rows, err := q.QueryContext(ctx, `SELECT e.id, e.at, e.customer_id, e.name, e.allowed, e.reason, e.source, COALESCE(u.name, '')
		FROM catraca_passagens e LEFT JOIN users u ON u.id = e.user_id
		WHERE e.at >= ? AND e.at < ? AND (? = '' OR e.name LIKE '%' || ? || '%') AND (? IS NULL OR e.allowed = ?)
		ORDER BY e.at, e.id`, FormatTime(from), FormatTime(to), term, term, allowed, allowed)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.GateEvent{}
	for rows.Next() {
		var (
			e   models.GateEvent
			at  string
			cid sql.NullInt64
		)
		if err := rows.Scan(&e.ID, &at, &cid, &e.Name, &e.Allowed, &e.Reason, &e.Source, &e.UserName); err != nil {
			return nil, wrap(op, err)
		}
		e.CustomerID = nullInt(cid)
		if e.At, err = parseTime(at); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, e)
	}
	return out, wrap(op, rows.Err())
}

// TodayCounts: liberados e negados desde from (meia-noite local).
func (CatracaRepo) TodayCounts(ctx context.Context, q DBTX, from time.Time) (allowed, denied int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT COALESCE(SUM(allowed), 0), COALESCE(SUM(1 - allowed), 0) FROM catraca_passagens WHERE at >= ?`,
		FormatTime(from)).Scan(&allowed, &denied)
	return allowed, denied, wrap("contar passagens", err)
}
