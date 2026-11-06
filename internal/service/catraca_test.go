package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"pdv/internal/models"
)

func TestGateDecide(t *testing.T) {
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.Local)
	plan := func(due string, paid, paused bool) models.GatePlan {
		return models.GatePlan{Name: "Mensal", DueDate: due, EverPaid: paid, Paused: paused}
	}
	cases := []struct {
		name  string
		m     models.GateMember
		tol   int64
		ok    bool
		maybe string // trecho do motivo
	}{
		{"sem plano", models.GateMember{Active: true}, 0, false, "Sem plano"},
		{"inativo", models.GateMember{Active: false, Plans: []models.GatePlan{plan("2026-11-20", true, false)}}, 0, false, "inativo"},
		{"bloqueado", models.GateMember{Active: true, Blocked: true, Plans: []models.GatePlan{plan("2026-11-20", true, false)}}, 0, false, "bloqueado"},
		{"em dia", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-11-20", true, false)}}, 0, true, "20/11/2026"},
		{"vence hoje e já pagou antes", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-10-28", true, false)}}, 0, true, "em dia"},
		{"vencida sem tolerância", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-10-27", true, false)}}, 0, false, "vencida em 27/10/2026"},
		{"vencida dentro da tolerância", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-10-26", true, false)}}, 2, true, "tolerância"},
		{"vencida fora da tolerância", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-10-25", true, false)}}, 2, false, "vencida"},
		{"primeira mensalidade em aberto", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-10-28", false, false)}}, 5, false, "Primeira mensalidade"},
		{"importada com vencimento futuro", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-11-05", false, false)}}, 0, true, "em dia"},
		{"pausada", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-11-20", true, true)}}, 0, false, "pausada"},
		{"cortesia", models.GateMember{Active: true, FreeUntil: "2026-10-28"}, 0, true, "Cortesia"},
		{"cortesia vencida", models.GateMember{Active: true, FreeUntil: "2026-10-27"}, 0, false, "Sem plano"},
		{"cortesia não fura bloqueio", models.GateMember{Active: true, Blocked: true, FreeUntil: "2026-12-01"}, 0, false, "bloqueado"},
		{"um plano em dia basta", models.GateMember{Active: true, Plans: []models.GatePlan{plan("2026-01-01", true, false), plan("2026-11-02", true, false)}}, 0, true, "02/11/2026"},
	}
	for _, c := range cases {
		ok, reason := GateDecide(c.m, now, c.tol)
		if ok != c.ok || !strings.Contains(reason, c.maybe) {
			t.Errorf("%s: esperava %v com %q, veio %v %q", c.name, c.ok, c.maybe, ok, reason)
		}
	}
}

func TestGateWithSubscription(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	subs := NewSubscriptionService(b)
	gate := NewCatracaService(b)
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone} // sem aparelho nos testes
	var events []models.GateEvent
	gate.OnEvent = func(ev models.GateEvent) { events = append(events, ev) }

	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	ana := e.customer("Ana", 0)

	check := func(want bool, part string) {
		t.Helper()
		res, err := gate.CheckAndRelease(e.ctx, ana.ID)
		if err != nil || res.Allowed != want || !strings.Contains(res.Reason, part) || res.Released {
			t.Fatalf("esperava %v com %q: %v %+v", want, part, err, res)
		}
	}
	check(false, "Sem plano")

	sub, err := subs.Create(e.ctx, ana.ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	check(false, "Primeira mensalidade")

	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix"}); err != nil {
		t.Fatal(err)
	}
	check(true, "em dia")

	// Pagamento atrasado: a nova mensalidade venceu há 3 dias.
	old := time.Now().AddDate(0, 0, -3).Format(dateLayout)
	if _, err := e.db.Exec(`UPDATE subscription_invoices SET due_date = ? WHERE status = 'aberta'`, old); err != nil {
		t.Fatal(err)
	}
	check(false, "vencida")
	gate.cfg.ToleranceDays = 5
	check(true, "tolerância")
	gate.cfg.ToleranceDays = 0

	// Cortesia libera; bloqueio barra mesmo com cortesia.
	free := time.Now().AddDate(0, 0, 7).Format(dateLayout)
	if m, err := gate.SaveMember(e.ctx, models.GateMemberInput{CustomerID: ana.ID, FreeUntil: free}); err != nil || !m.Allowed {
		t.Fatalf("cortesia deveria liberar: %v %+v", err, m)
	}
	if m, err := gate.SaveMember(e.ctx, models.GateMemberInput{CustomerID: ana.ID, FreeUntil: free, Blocked: true}); err != nil || m.Allowed {
		t.Fatalf("bloqueio deveria barrar: %v %+v", err, m)
	}
	if _, err := gate.SaveMember(e.ctx, models.GateMemberInput{CustomerID: ana.ID, FreeUntil: "28/10/2026"}); codeOf(err) != CodeInvalid {
		t.Fatalf("data inválida: esperava OPERACAO_INVALIDA, veio %v", err)
	}

	// Operador sem a permissão da catraca não bloqueia nem dá cortesia.
	op := e.user("op", models.RoleOperator)
	if _, err := gate.SaveMember(e.as(op), models.GateMemberInput{CustomerID: ana.ID}); err == nil {
		t.Fatal("operador sem permissão não deveria alterar a catraca")
	}

	// Lista: a Ana aparece sem busca (tem plano) e com busca.
	if list, err := gate.Members(e.ctx, ""); err != nil || len(list) != 1 || list[0].CustomerID != ana.ID || len(list[0].Plans) != 1 {
		t.Fatalf("lista sem busca: %v %+v", err, list)
	}
	e.customer("Bruno", 0)
	if list, _ := gate.Members(e.ctx, "bru"); len(list) != 1 || list[0].Name != "Bruno" || list[0].Allowed {
		t.Fatalf("busca deveria achar o Bruno, sem plano: %+v", list)
	}

	// Manual: o motivo é opcional; passagens ficam registradas com o operador.
	if err := gate.ManualRelease(e.ctx, "  "); err != nil {
		t.Fatalf("liberação sem motivo: %v", err)
	}
	if got := events[len(events)-1]; got.Source != "manual" || got.Reason != manualNoReason {
		t.Fatalf("sem motivo registrado como: %+v", got)
	}
	if err := gate.ManualRelease(e.ctx, "Aula experimental"); err != nil {
		t.Fatal(err)
	}
	list, err := gate.Events(e.ctx, 100)
	if err != nil || len(list) != len(events) || len(list) < 6 || list[0].Source != "manual" || list[0].UserName != "Admin" {
		t.Fatalf("passagens: %v %d/%d %+v", err, len(list), len(events), list[0])
	}
	if st, err := gate.Status(e.ctx); err != nil || st.AllowedToday+st.DeniedToday != int64(len(list)) || st.Config.DevicePassword != "" {
		t.Fatalf("contagem do dia: %v %+v", err, st)
	}

	// Relatório do dia: tudo, só a manual (busca vazia pelo nome) e período inválido.
	day := time.Now().Format(dateLayout)
	if rep, err := gate.EventReport(e.ctx, day, day, "", ""); err != nil || len(rep) != len(list) || rep[len(rep)-1].Source != "manual" {
		t.Fatalf("relatório do dia: %v %d/%d", err, len(rep), len(list))
	}
	var denied int
	for _, ev := range list {
		if !ev.Allowed {
			denied++
		}
	}
	if rep, err := gate.EventReport(e.ctx, day, day, "", "negadas"); err != nil || len(rep) != denied {
		t.Fatalf("negadas: %v %d/%d", err, len(rep), denied)
	}
	if rep, err := gate.EventReport(e.ctx, day, day, "zzz", ""); err != nil || len(rep) != 0 {
		t.Fatalf("busca sem resultado: %v %d", err, len(rep))
	}
	tomorrow := time.Now().AddDate(0, 0, 1).Format(dateLayout)
	if rep, _ := gate.EventReport(e.ctx, tomorrow, tomorrow, "", ""); len(rep) != 0 {
		t.Fatalf("amanhã não tem passagens: %d", len(rep))
	}
	if _, err := gate.EventReport(e.ctx, tomorrow, day, "", ""); codeOf(err) != CodeInvalid {
		t.Fatalf("período invertido: %v", err)
	}
}

func TestGateStaffAndReactivate(t *testing.T) {
	e := newEnv(t)
	b := NewBase(e.db)
	gate := NewCatracaService(b)
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone}

	// Funcionário novo passa sem plano e sai da lista de alunos.
	jo, err := gate.SaveStaff(e.ctx, models.GateStaffInput{Name: "Joseane Silva", Role: "Recepção"})
	if err != nil || !jo.Staff || !jo.Allowed || jo.Reason != "Funcionário · Recepção" {
		t.Fatalf("funcionária nova: %v %+v", err, jo)
	}
	// Cliente inativo vira funcionário: o cadastro é ativado.
	ma := e.customer("Mateus", 0)
	if _, err := e.db.Exec(`UPDATE customers SET active = 0 WHERE id = ?`, ma.ID); err != nil {
		t.Fatal(err)
	}
	if m, err := gate.SaveStaff(e.ctx, models.GateStaffInput{CustomerID: ma.ID}); err != nil || !m.Active || !m.Allowed {
		t.Fatalf("cliente inativo como funcionário: %v %+v", err, m)
	}
	if list, err := gate.Staff(e.ctx); err != nil || len(list) != 2 {
		t.Fatalf("lista de funcionários: %v %+v", err, list)
	}
	if m, err := gate.SaveMember(e.ctx, models.GateMemberInput{CustomerID: jo.CustomerID, Blocked: true}); err != nil || m.Allowed {
		t.Fatalf("funcionário bloqueado deveria ser barrado: %v %+v", err, m)
	}
	if list, _ := gate.Members(e.ctx, ""); len(list) != 0 {
		t.Fatalf("funcionário não é aluno: %+v", list)
	}
	if m, err := gate.RemoveStaff(e.ctx, ma.ID); err != nil || m.Staff || m.Allowed {
		t.Fatalf("desligado volta a ser cliente sem plano: %v %+v", err, m)
	}
	op := e.user("op", models.RoleOperator)
	if _, err := gate.SaveStaff(e.as(op), models.GateStaffInput{Name: "X"}); err == nil {
		t.Fatal("operador sem permissão não cadastra funcionário")
	}

	// Reativar: a mesma assinatura volta e o cliente inativo é ativado. Nunca foi paga:
	// a mensalidade de antes (vence hoje) volta em aberto e ainda se paga antes de entrar.
	subs := NewSubscriptionService(b)
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	ana := e.customer("Ana", 0)
	sub, err := subs.Create(e.ctx, ana.ID, plan.ID, 8500, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subs.Reactivate(e.ctx, sub.ID); codeOf(err) != CodeStateConflict {
		t.Fatalf("só reativa cancelada: %v", err)
	}
	if _, err := subs.Cancel(e.ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE customers SET active = 0 WHERE id = ?`, ana.ID); err != nil {
		t.Fatal(err)
	}
	re, err := subs.Reactivate(e.ctx, sub.ID)
	if err != nil || re.Status != "ativa" || re.ID != sub.ID || re.OpenInvoice == nil || re.OpenInvoice.AmountCents != 8500 ||
		re.OpenInvoice.DueDate != time.Now().Format(dateLayout) {
		t.Fatalf("reativar: %v %+v", err, re)
	}
	if m, _ := gate.member(e.ctx, ana.ID); !m.Active || m.Allowed || !strings.Contains(m.Reason, "Primeira mensalidade") {
		t.Fatalf("reativada: cliente ativo, mas paga antes de entrar: %+v", m)
	}
}

func TestGateRules(t *testing.T) {
	// 28/10/2026 é uma quarta-feira (3).
	now := time.Date(2026, 10, 28, 15, 0, 0, 0, time.Local)
	ok := func(r models.AccessRules, week, today int64) models.GateMember {
		return models.GateMember{Active: true, DaysThisWeek: week, EntriesToday: today,
			Plans: []models.GatePlan{{Name: "Mensal", DueDate: "2026-11-20", EverPaid: true, Rules: r}}}
	}
	cases := []struct {
		name  string
		m     models.GateMember
		pass  bool
		maybe string
	}{
		{"sem regras", ok(models.AccessRules{}, 6, 9), true, "em dia"},
		{"dia permitido", ok(models.AccessRules{Weekdays: []int64{1, 3, 5}}, 0, 0), true, "em dia"},
		{"dia não permitido", ok(models.AccessRules{Weekdays: []int64{2, 4}}, 0, 0), false, "(ter, qui)"},
		{"semana cheia", ok(models.AccessRules{WeeklyLimit: 3}, 3, 0), false, "3x na semana"},
		{"semana cheia, mas já entrou hoje", ok(models.AccessRules{WeeklyLimit: 3}, 3, 1), true, "em dia"},
		{"limite do dia", ok(models.AccessRules{DailyLimit: 2}, 1, 2), false, "2 entrada(s) por dia"},
		{"dentro do limite do dia", ok(models.AccessRules{DailyLimit: 2}, 1, 1), true, "em dia"},
		{"outro plano sem regra libera", models.GateMember{Active: true, Plans: []models.GatePlan{
			{Name: "Seg", DueDate: "2026-11-20", EverPaid: true, Rules: models.AccessRules{Weekdays: []int64{1}}},
			{Name: "Livre", DueDate: "2026-11-21", EverPaid: true}}}, true, "Livre"},
		{"vencida vale mais que a regra", models.GateMember{Active: true, Plans: []models.GatePlan{
			{Name: "Mensal", DueDate: "2026-10-01", EverPaid: true, Rules: models.AccessRules{Weekdays: []int64{1}}}}}, false, "vencida"},
	}
	for _, c := range cases {
		pass, reason := GateDecide(c.m, now, 0)
		if pass != c.pass || !strings.Contains(reason, c.maybe) {
			t.Errorf("%s: esperava %v com %q, veio %v %q", c.name, c.pass, c.maybe, pass, reason)
		}
	}
	for _, bad := range []models.AccessRules{{Weekdays: []int64{0}}, {WeeklyLimit: 8}, {DailyLimit: -1}, {Weekdays: []int64{1, 3}, WeeklyLimit: 3}} {
		if _, err := validateRules(bad); codeOf(err) != CodeInvalid {
			t.Errorf("regra inválida aceita: %+v", bad)
		}
	}
	if r, err := validateRules(models.AccessRules{Weekdays: []int64{5, 1, 5}}); err != nil || len(r.Weekdays) != 2 || r.Weekdays[0] != 1 {
		t.Errorf("dias deveriam sair sem repetir e em ordem: %v %+v", err, r)
	}
}

func TestGateRulesCountEntries(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	subs := NewSubscriptionService(b)
	gate := NewCatracaService(b)
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone}
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	ana := e.customer("Ana", 0)
	sub, err := subs.Create(e.ctx, ana.ID, plan.ID, 0, &models.AccessRules{DailyLimit: 2, WeeklyLimit: 3})
	if err != nil || sub.Rules.DailyLimit != 2 || sub.Rules.WeeklyLimit != 3 {
		t.Fatalf("assinatura com regras: %v %+v", err, sub.Rules)
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix"}); err != nil {
		t.Fatal(err)
	}
	// Leitor: a repetição em 10 s devolve a mesma resposta e não gasta entrada.
	id := int64(5)
	if err := gate.repo.SetDevice(e.ctx, e.db, ana.ID, &id, time.Now()); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if d := gate.identified(e.ctx, 5); !d.Allowed {
			t.Fatalf("1ª entrada do dia deveria passar: %+v", d)
		}
	}
	if res, err := gate.CheckAndRelease(e.ctx, ana.ID); err != nil || !res.Allowed {
		t.Fatalf("2ª entrada do dia deveria passar: %v %+v", err, res)
	}
	if res, err := gate.CheckAndRelease(e.ctx, ana.ID); err != nil || res.Allowed || !strings.Contains(res.Reason, "por dia") {
		t.Fatalf("3ª entrada do dia deveria ser negada: %v %+v", err, res)
	}
	// Tirar o limite do dia libera de novo; as regras aparecem na assinatura.
	if s, err := subs.SetRules(e.ctx, sub.ID, &models.AccessRules{WeeklyLimit: 3}); err != nil || s.Rules.DailyLimit != 0 {
		t.Fatalf("trocar regras: %v %+v", err, s.Rules)
	}
	if res, _ := gate.CheckAndRelease(e.ctx, ana.ID); !res.Allowed {
		t.Fatalf("sem limite do dia deveria passar: %+v", res)
	}
	if m, _ := gate.member(e.ctx, ana.ID); m.EntriesToday != 3 || m.DaysThisWeek != 1 {
		t.Fatalf("uso da semana: hoje %d, dias %d", m.EntriesToday, m.DaysThisWeek)
	}
	// Reativar leva as regras.
	if _, err := subs.Cancel(e.ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	if re, err := subs.Reactivate(e.ctx, sub.ID); err != nil || re.Rules.WeeklyLimit != 3 {
		t.Fatalf("reativar com as regras: %v %+v", err, re.Rules)
	}
}

func TestGateRemoveMember(t *testing.T) {
	e := newEnv(t)
	b := NewBase(e.db)
	gate := NewCatracaService(b)
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone} // sem aparelho: o rosto sai só daqui
	subs := NewSubscriptionService(b)
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	bia := e.customer("Bia", 0)
	sub, err := subs.Create(e.ctx, bia.ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := int64(9)
	if err := gate.repo.SetDevice(e.ctx, e.db, bia.ID, &id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := gate.RemoveMember(e.as(e.user("op", models.RoleOperator)), bia.ID); err == nil {
		t.Fatal("operador sem permissão não apaga aluno")
	}
	if err := gate.RemoveMember(e.ctx, bia.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := gate.Members(e.ctx, ""); len(list) != 0 {
		t.Fatalf("apagada deveria sumir da lista: %+v", list)
	}
	if m, _ := gate.member(e.ctx, bia.ID); m.Active || m.DeviceUserID != nil || len(m.Plans) != 0 {
		t.Fatalf("apagada: inativa, sem rosto e sem plano: %+v", m)
	}
	if s, _ := subs.get(e.ctx, sub.ID); s.Status != "cancelada" || s.OpenInvoice != nil {
		t.Fatalf("assinatura deveria estar cancelada: %+v", s)
	}
	if re, err := subs.Reactivate(e.ctx, sub.ID); err != nil || re.Status != "ativa" {
		t.Fatalf("reativar depois de apagar: %v %+v", err, re)
	}
}

func TestCustomerActiveAndDelete(t *testing.T) {
	e := newEnv(t)
	b := NewBase(e.db)
	gate := NewCatracaService(b)
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone}
	custs := NewCustomerService(b)
	ana := e.customer("Ana", 0)
	if c, err := custs.SetActive(e.ctx, ana.ID, false); err != nil || c.Active {
		t.Fatalf("inativar: %v %+v", err, c)
	}
	if c, err := custs.SetActive(e.ctx, ana.ID, true); err != nil || !c.Active {
		t.Fatalf("ativar: %v %+v", err, c)
	}
	// Sem histórico (só com rosto ligado): exclui, junto com a linha da catraca.
	id := int64(3)
	if err := gate.repo.SetDevice(e.ctx, e.db, ana.ID, &id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := gate.DeleteCustomer(e.ctx, ana.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := custs.Get(e.ctx, ana.ID); codeOf(err) != CodeCustomerNotFound {
		t.Fatalf("excluída deveria sumir: %v", err)
	}
	if n := e.count("catraca_alunos"); n != 0 {
		t.Fatalf("linha da catraca deveria sair junto: %d", n)
	}
	// Com assinatura: recusa e nada muda.
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	bia := e.customer("Bia", 0)
	if _, err := NewSubscriptionService(b).Create(e.ctx, bia.ID, plan.ID, 0, nil); err != nil {
		t.Fatal(err)
	}
	if err := gate.DeleteCustomer(e.ctx, bia.ID); codeOf(err) != CodeStateConflict || !strings.Contains(err.Error(), "Inativar") {
		t.Fatalf("com histórico deveria recusar: %v", err)
	}
	if _, err := custs.Get(e.ctx, bia.ID); err != nil {
		t.Fatalf("a Bia deveria continuar: %v", err)
	}
}

func TestGatePlanRules(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	subs := NewSubscriptionService(b)
	gate := NewCatracaService(b)
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone}
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "3x na semana", PriceCents: 7500, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	if r, err := subs.SetPlanRules(e.ctx, plan.ID, models.AccessRules{DailyLimit: 1}); err != nil || r.DailyLimit != 1 {
		t.Fatalf("regras do plano: %v %+v", err, r)
	}
	if all, err := subs.PlanRules(e.ctx); err != nil || all[plan.ID].DailyLimit != 1 {
		t.Fatalf("listar regras dos planos: %v %+v", err, all)
	}
	comum, err := e.products.Create(e.ctx, models.ProductInput{Name: "Água", PriceCents: 300, Active: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subs.SetPlanRules(e.ctx, comum.ID, models.AccessRules{DailyLimit: 1}); codeOf(err) != CodeInvalid {
		t.Fatalf("produto que não é plano não tem regras: %v", err)
	}
	ana := e.customer("Ana", 0)
	sub, err := subs.Create(e.ctx, ana.ID, plan.ID, 0, nil)
	if err != nil || sub.Rules != nil {
		t.Fatalf("assinatura sem exceção segue o plano: %v %+v", err, sub.Rules)
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix"}); err != nil {
		t.Fatal(err)
	}
	if res, _ := gate.CheckAndRelease(e.ctx, ana.ID); !res.Allowed {
		t.Fatalf("1ª entrada passa: %+v", res)
	}
	if res, _ := gate.CheckAndRelease(e.ctx, ana.ID); res.Allowed || !strings.Contains(res.Reason, "1 entrada(s) por dia") {
		t.Fatalf("o limite do plano vale para a assinatura: %+v", res)
	}
	// Exceção sem restrição: a Ana fica livre, mesmo no plano limitado.
	if s, err := subs.SetRules(e.ctx, sub.ID, &models.AccessRules{}); err != nil || s.Rules == nil {
		t.Fatalf("exceção livre: %v %+v", err, s.Rules)
	}
	if res, _ := gate.CheckAndRelease(e.ctx, ana.ID); !res.Allowed {
		t.Fatalf("com a exceção, passa: %+v", res)
	}
	// Sem exceção de novo: volta a valer o plano.
	if s, err := subs.SetRules(e.ctx, sub.ID, nil); err != nil || s.Rules != nil {
		t.Fatalf("voltar ao plano: %v %+v", err, s.Rules)
	}
	if res, _ := gate.CheckAndRelease(e.ctx, ana.ID); res.Allowed {
		t.Fatalf("de volta ao plano, o limite do dia barra: %+v", res)
	}
}

func TestGateIdentifiedByDevice(t *testing.T) {
	e := newEnv(t)
	gate := NewCatracaService(NewBase(e.db))
	gate.cfg = models.GateConfig{Relay: models.GateRelayNone}
	if d := gate.identified(e.ctx, 77); d.Allowed || !strings.Contains(d.Message, "sem cadastro") {
		t.Fatalf("rosto desconhecido deveria ser negado: %+v", d)
	}
	ana := e.customer("Ana Souza", 0)
	id := int64(77)
	if err := gate.repo.SetDevice(e.ctx, e.db, ana.ID, &id, time.Now()); err != nil {
		t.Fatal(err)
	}
	free := time.Now().Format(dateLayout)
	if _, err := gate.SaveMember(e.ctx, models.GateMemberInput{CustomerID: ana.ID, FreeUntil: free}); err != nil {
		t.Fatal(err)
	}
	d := gate.identified(e.ctx, 77)
	if !d.Allowed || d.Name != "Ana Souza" || d.Message != "Bem-vindo(a), Ana!" {
		t.Fatalf("aluna com cortesia deveria passar: %+v", d)
	}
	// O leitor repete a identificação: só uma passagem registrada em 10 s.
	gate.identified(e.ctx, 77)
	if n := e.count("catraca_passagens"); n != 2 {
		t.Fatalf("esperava 2 passagens (desconhecido + Ana), veio %d", n)
	}
}

func freePort(t *testing.T) int64 {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return int64(ln.Addr().(*net.TCPAddr).Port)
}

// O iDFace chama o modo online: quem pode passa (com a ação do SecBox), quem
// não pode e o desconhecido são negados, e tudo fica registrado.
func TestGateOnlineMode(t *testing.T) {
	e := newEnv(t)
	gate := NewCatracaService(NewBase(e.db))
	t.Cleanup(gate.Stop)
	cfg := defaultGateConfig()
	cfg.ServerPort, cfg.DeviceHost = freePort(t), "127.0.0.1:1"
	if _, err := gate.SaveConfig(e.ctx, cfg); err != nil {
		t.Fatal(err)
	}
	var live []models.GateEvent
	gate.OnEvent = func(ev models.GateEvent) { live = append(live, ev) }
	now := time.Now()
	ana, bia := e.customer("Ana Souza", 0), e.customer("Bia", 0)
	ten, eleven := int64(10), int64(11)
	gate.repo.SetDevice(e.ctx, e.db, ana.ID, &ten, now)
	gate.repo.SetDevice(e.ctx, e.db, bia.ID, &eleven, now)
	if _, err := gate.SaveMember(e.ctx, models.GateMemberInput{CustomerID: ana.ID, FreeUntil: now.AddDate(0, 0, 5).Format(dateLayout)}); err != nil {
		t.Fatal(err)
	}

	ask := func(userID int64, form bool) map[string]any {
		addr := "http://127.0.0.1:" + strconv.FormatInt(cfg.ServerPort, 10) + "/new_user_identified.fcgi"
		var res *http.Response
		var err error
		if form {
			res, err = http.PostForm(addr, url.Values{"user_id": {strconv.FormatInt(userID, 10)}, "portal_id": {"1"}})
		} else {
			res, err = http.Post(addr, "application/json", strings.NewReader(`{"user_id":`+strconv.FormatInt(userID, 10)+`,"portal_id":1}`))
		}
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out struct{ Result map[string]any }
		json.NewDecoder(res.Body).Decode(&out)
		return out.Result
	}

	r := ask(10, true)
	acts, _ := r["actions"].([]any)
	if r["event"] != 7.0 || len(acts) != 1 || !strings.Contains(acts[0].(map[string]any)["parameters"].(string), "id=65793") {
		t.Fatalf("Ana com cortesia deveria passar com a ação do SecBox: %+v", r)
	}
	if r := ask(10, false); r["event"] != 7.0 {
		t.Fatalf("corpo em JSON também deveria funcionar: %+v", r)
	}
	if r := ask(11, true); r["event"] != 6.0 || r["message"] != "Sem plano ativo" {
		t.Fatalf("Bia sem plano deveria ser negada: %+v", r)
	}
	if r := ask(99, true); r["event"] != 6.0 {
		t.Fatalf("rosto sem cadastro deveria ser negado: %+v", r)
	}
	// A segunda identificação da Ana em menos de 10 s não gera outro registro.
	if len(live) != 3 || !live[0].Allowed || live[1].Allowed || live[2].CustomerID != nil {
		t.Fatalf("registros ao vivo: %+v", live)
	}
	if m, _ := gate.member(e.ctx, ana.ID); m.LastAccess == nil {
		t.Fatal("último acesso da Ana deveria estar preenchido")
	}
}

// Importa as pessoas de um iDFace falso: liga pela matrícula C<código>, depois
// pelo nome igual, e cria cliente para o resto.
func TestGateImportFromDevice(t *testing.T) {
	e := newEnv(t)
	carla := e.customer("Carla Lima", 0)
	e.customer("Davi Rocha", 0)
	users := fmt.Sprintf(`{"users":[{"id":1,"name":"Carla","registration":"C%d"},{"id":2,"name":"davi  rocha","registration":"992"},{"id":3,"name":"","registration":""}]}`, carla.ID)
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		switch r.URL.Path {
		case "/login.fcgi":
			io.WriteString(w, `{"session":"abc"}`)
		case "/load_objects.fcgi":
			if r.URL.Query().Get("session") != "abc" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			io.WriteString(w, users)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer fake.Close()

	gate := NewCatracaService(NewBase(e.db))
	t.Cleanup(gate.Stop)
	cfg := defaultGateConfig()
	cfg.ServerPort, cfg.DeviceHost = freePort(t), strings.TrimPrefix(fake.URL, "http://")
	if _, err := gate.SaveConfig(e.ctx, cfg); err != nil {
		t.Fatal(err)
	}
	res, err := gate.ImportFromDevice(e.ctx)
	if err != nil || res.DeviceUsers != 3 || res.Linked != 2 || res.Created != 1 {
		t.Fatalf("importação: %v %+v", err, res)
	}
	if m, _ := gate.member(e.ctx, carla.ID); m.DeviceUserID == nil || *m.DeviceUserID != 1 {
		t.Fatalf("Carla deveria ficar ligada ao usuário 1 pela matrícula: %+v", m)
	}
	if again, _ := gate.ImportFromDevice(e.ctx); again.Already != 3 || again.Created != 0 {
		t.Fatalf("importar de novo não deveria duplicar: %+v", again)
	}
	if n, err := gate.TestDevice(e.ctx); err != nil || n != 3 {
		t.Fatalf("teste do aparelho: %v %d", err, n)
	}
}

// Cancelou por engano quem já tinha pago: reativar devolve a mesma assinatura com o
// vencimento de antes, sem cobrar de novo, e a catraca libera. Excluir a cancelada a
// tira da lista sem apagar as mensalidades pagas.
func TestReactivatePaidAndDelete(t *testing.T) {
	e := newEnv(t).withCash()
	b := NewBase(e.db)
	subs, gate := NewSubscriptionService(b), NewCatracaService(b)
	days := int64(30)
	plan, err := e.products.Create(e.ctx, models.ProductInput{Name: "Mensal", PriceCents: 9000, Active: true, PlanDays: &days})
	if err != nil {
		t.Fatal(err)
	}
	ana := e.customer("Ana", 0)
	sub, err := subs.Create(e.ctx, ana.ID, plan.ID, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subs.Pay(e.ctx, models.PayInvoiceInput{InvoiceID: sub.OpenInvoice.ID, PaymentMethod: "pix"}); err != nil {
		t.Fatal(err)
	}
	paid, _ := subs.get(e.ctx, sub.ID)
	due := paid.OpenInvoice.DueDate
	if err := subs.Delete(e.ctx, sub.ID); codeOf(err) != CodeStateConflict {
		t.Fatalf("só exclui cancelada: %v", err)
	}
	if _, err := subs.Cancel(e.ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	re, err := subs.Reactivate(e.ctx, sub.ID)
	if err != nil || re.ID != sub.ID || re.OpenInvoice == nil || re.OpenInvoice.ID != paid.OpenInvoice.ID || re.OpenInvoice.DueDate != due {
		t.Fatalf("reativar deveria voltar a mensalidade de %s: %v %+v", due, err, re.OpenInvoice)
	}
	if m, _ := gate.member(e.ctx, ana.ID); !m.Allowed {
		t.Fatalf("já tinha pago: deveria passar: %+v", m)
	}
	if invs, _ := subs.Invoices(e.ctx, sub.ID); len(invs) != 2 {
		t.Fatalf("a paga e a em aberto: %+v", invs)
	}

	// Excluir: só cancelada, com a permissão; some da lista e não reativa mais.
	if _, err := subs.Cancel(e.ctx, sub.ID); err != nil {
		t.Fatal(err)
	}
	if err := subs.Delete(e.as(e.user("op", models.RoleOperator)), sub.ID); err == nil {
		t.Fatal("operador sem a permissão não exclui")
	}
	if n, err := subs.DeleteCancelled(e.ctx, ana.ID); err != nil || n != 1 {
		t.Fatalf("excluir canceladas: %d %v", n, err)
	}
	if list, _ := subs.List(e.ctx); len(list) != 0 {
		t.Fatalf("excluída deveria sumir da lista: %+v", list)
	}
	if _, err := subs.Reactivate(e.ctx, sub.ID); codeOf(err) != CodeStateConflict {
		t.Fatalf("excluída não reativa: %v", err)
	}
	var kept int
	e.db.QueryRow(`SELECT COUNT(*) FROM subscription_invoices WHERE subscription_id = ? AND status = 'paga'`, sub.ID).Scan(&kept)
	if kept != 1 {
		t.Fatalf("a mensalidade paga continua guardada: %d", kept)
	}
}
