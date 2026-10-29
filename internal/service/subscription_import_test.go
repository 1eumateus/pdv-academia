package service

import (
	"encoding/base64"
	"testing"
)

func TestImportHelpers(t *testing.T) {
	if got := titleCase("ADINALDA DOS SANTOS E SILVA"); got != "Adinalda dos Santos e Silva" {
		t.Errorf("titleCase: %q", got)
	}
	for in, want := range map[string]string{"2026-04-07": "2026-04-07", "07/04/2026": "2026-04-07", "2026-10-22 22:04:23": "2026-10-22", "46119": "2026-04-07", "x": ""} {
		if got := parseSheetDate(in); got != want {
			t.Errorf("parseSheetDate(%q) = %q, esperava %q", in, got, want)
		}
	}
	rows := [][]string{{"MUSCULA��O", "Jo�o da Silva", "RUA S�O JOSÉ"}, {"MUSCULAÇÃO"}}
	if n := fixReplacementChars(rows); n != 3 || rows[0][0] != "MUSCULAÇÃO" || rows[0][1] != "João da Silva" || rows[0][2] != "RUA SÃO JOSÉ" {
		t.Fatalf("correção do �: %d %q", n, rows[0])
	}
	for _, k := range []string{"clientes", "mensalidades"} {
		if _, err := SubscriptionTemplate(k); err != nil {
			t.Fatal(k, err)
		}
	}
}

func TestSubscriptionImport(t *testing.T) {
	e := newEnv(t)
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	// Ana e Bruno com histórico; Carla só no modo simples (plano trimestral, 90 dias).
	clients := "Código (obrigatório);Nome (obrigatório);Nascimento;Celular;Ativo (Sim/Não);Plano;Duração (dias);Valor;Próximo vencimento\n" +
		"308;ANA BEATRIZ;1990-01-01;94984502770;Sim;;;;\n" +
		"309;BRUNO LIMA;1991-02-02;94984502770;;;;;\n" +
		"310;CARLA DIAS;;94984502770;Sim;Trimestral;90;250,00;05/12/2026\n"
	// Ana pagou adiantado (em 20/08) a de 20/10. Bruno tem uma em aberto de 19/09, mas pagou a de 19/10.
	invoices := "Código;Plano;Vencimento;Valor;Pago (Sim/Não);Pago em\n" +
		"308;Mensal;2026-09-20;95,00;Sim;2026-08-20\n" +
		"308;mensal;2026-10-20;170,00;Sim;2026-08-20\n" +
		"309;Mensal;2026-09-19;100,00;Não;\n" +
		"309;Mensal;2026-10-19;100,00;Sim;2026-10-19\n"
	s := NewSubscriptionImportService(NewBase(e.db))
	res, err := s.Import(e.ctx, "c.csv", b64(clients), "f.csv", b64(invoices), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plans) != 2 {
		t.Fatalf("esperava 2 planos (Mensal e Trimestral): %+v", res.Plans)
	}
	list, _ := NewSubscriptionService(NewBase(e.db)).List(e.ctx)
	want := map[string]string{"Ana Beatriz": "2026-11-19", "Bruno Lima": "2026-11-18", "Carla Dias": "2026-12-05"}
	if len(list) != 3 {
		t.Fatalf("esperava 3 assinaturas: %d", len(list))
	}
	for _, sub := range list {
		if sub.OpenInvoice == nil || sub.OpenInvoice.DueDate != want[sub.CustomerName] {
			t.Errorf("%s: mensalidade em aberto %+v, esperava vencer em %s", sub.CustomerName, sub.OpenInvoice, want[sub.CustomerName])
		}
	}
	var old int
	e.db.QueryRow(`SELECT COUNT(*) FROM subscription_invoices WHERE status = 'nao_paga'`).Scan(&old)
	if old != 1 {
		t.Fatalf("a em aberto de 19/09 (antes de um pagamento mais novo) deveria ficar como não paga: %d", old)
	}

	// De novo: quem já tem assinatura é pulado, sem duplicar.
	res, err = s.Import(e.ctx, "c.csv", b64(clients), "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if list, _ = NewSubscriptionService(NewBase(e.db)).List(e.ctx); len(list) != 3 || res.CustomersWithSub != 1 {
		t.Fatalf("reimportar duplicou: %d assinaturas, %d puladas", len(list), res.CustomersWithSub)
	}
}
