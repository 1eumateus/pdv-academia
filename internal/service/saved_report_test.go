// Caminho: pdv/internal/service/saved_report_test.go

package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"

	"pdv/internal/models"
)

func TestSavedReportKeepsNumbersAndPrints(t *testing.T) {
	e := newEnv(t).withCash()
	caneta := e.product("Caneta", 500, 10)
	e.product("Régua", 300, 3) // sem venda e com estoque abaixo do padrão (5)
	if _, err := e.sell(models.PaymentCash, nil, item(caneta.ID, 2)); err != nil {
		t.Fatal(err)
	}
	r, err := e.reports.Generate(e.ctx, today(), today(), 0, models.ReportGeneral)
	if err != nil || r.ID == 0 || r.TotalCents != 1000 {
		t.Fatalf("gerar: %+v %v", r, err)
	}
	if len(r.NotSold) != 1 || r.NotSold[0].Name != "Régua" || r.NotSold[0].StockQty != 3 || r.LowStockDefault != 5 {
		t.Fatalf("sem venda: %+v padrão %d", r.NotSold, r.LowStockDefault)
	}
	if r.Products[0].StockQty != 8 {
		t.Fatalf("estoque atual no ranking: %+v", r.Products[0])
	}
	// Régua (3 un., sem mínimo) entra no alerta pelo padrão de 5.
	if _, low, _, err := e.reports.reports.StockAlerts(e.ctx, e.db, 5, 10); err != nil || low != 1 {
		t.Fatalf("alerta pelo padrão: low=%d %v", low, err)
	}
	// Venda depois de gerado não muda a cópia guardada.
	if _, err := e.sell(models.PaymentCash, nil, item(caneta.ID, 1)); err != nil {
		t.Fatal(err)
	}
	list, err := e.reports.ListSaved(e.ctx)
	if err != nil || len(list) != 1 || list[0].TotalCents != 1000 || list[0].SaleCount != 1 {
		t.Fatalf("lista: %+v %v", list, err)
	}
	rec, err := NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptReport, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	txt := receiptText(rec)
	for _, want := range []string{"RELATÓRIO DE VENDAS", "TOTAL VENDIDO R$ 10,00", "Caneta 2  10,00"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("comprovante sem %q:\n%s", want, txt)
		}
	}

	xlsx, err := e.reports.SavedXLSX(e.ctx, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(xlsx), int64(len(xlsx)))
	if err != nil {
		t.Fatal(err)
	}
	var sheet string
	for _, f := range z.File {
		rc, _ := f.Open()
		body, _ := io.ReadAll(rc)
		rc.Close()
		// Todo XML do pacote precisa ser válido, senão o Excel recusa o arquivo.
		for d := xml.NewDecoder(bytes.NewReader(body)); ; {
			if _, err := d.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s inválido: %v", f.Name, err)
			}
		}
		if f.Name == "xl/worksheets/sheet1.xml" {
			sheet = string(body)
		}
	}
	for _, want := range []string{"Relatório de vendas", "Total vendido", "<v>10.00</v>", "Caneta"} {
		if !strings.Contains(sheet, want) {
			t.Fatalf("planilha sem %q:\n%s", want, sheet)
		}
	}

	// Relatório só de uma seção: imprime aquela tabela e não as outras.
	cat, err := e.reports.Generate(e.ctx, today(), today(), 0, models.ReportByCategory)
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := e.reports.ListSaved(e.ctx); l[0].Kind != models.ReportByCategory || l[1].Kind != models.ReportGeneral {
		t.Fatalf("tipos na lista: %+v", l)
	}
	rec, err = NewReceiptService(NewBase(e.db)).Build(e.ctx, models.ReceiptReport, cat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if txt := receiptText(rec); !strings.Contains(txt, "VENDAS POR CATEGORIA") || !strings.Contains(txt, "Por categoria:") ||
		strings.Contains(txt, "Por forma de pagamento:") || strings.Contains(txt, "Produtos vendidos:") {
		t.Fatalf("relatório por categoria:\n%s", txt)
	}
	if _, err := e.reports.Generate(e.ctx, today(), today(), 0, "qualquer"); err == nil {
		t.Fatal("aceitou tipo desconhecido")
	}

	// Operador sem a permissão não vê relatório gerado por outro.
	op := e.user("caixa", models.RoleOperator)
	if l, _ := e.reports.ListSaved(e.as(op)); len(l) != 0 {
		t.Fatalf("operador viu relatórios de outro: %+v", l)
	}
	if _, _, err := e.reports.Saved(e.as(op), r.ID); err == nil {
		t.Fatal("operador abriu relatório de outro")
	}
}
