// Caminho: pdv/internal/service/nfe_import_test.go

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pdv/internal/models"
)

const sampleNFe = "../nfe/testdata/nfeproc_compra.xml"

// nfeVariant grava uma cópia do XML de exemplo com uma troca de texto.
func nfeVariant(t *testing.T, old, new string) string {
	t.Helper()
	data, err := os.ReadFile(sampleNFe)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "nota.xml")
	if err := os.WriteFile(p, []byte(strings.Replace(string(data), old, new, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNFeImport(t *testing.T) {
	e := newEnv(t)
	svc := NewNFeService(NewBase(e.db), e.dataDir)
	ean := "7891234567895"
	caneta, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Caneta azul", Barcode: &ean, PriceCents: 500, Active: true})
	caderno := e.product("Caderno 10 matérias", 2500, 0)

	pv, err := svc.PreviewFile(e.ctx, sampleNFe)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Supplier.ID != 0 || pv.Supplier.Document != "11222333000181" || pv.AlreadyImported || len(pv.Items) != 2 {
		t.Fatalf("conferência inicial: %+v", pv)
	}
	a, b := pv.Items[0], pv.Items[1]
	// Item 1: 25,00 + frete 2,00 + IPI 1,00 = 28,00 por 10 un → 2,80
	if a.MatchedBy != "codigo_barras" || *a.ProductID != caneta.ID || a.Units != 10 || a.UnitCostCents != 280 || a.Problem != "" {
		t.Fatalf("item 1: %+v", a)
	}
	if b.ProductID != nil || b.Problem == "" || b.QtyText != "2" || b.Unit != "CX" {
		t.Fatalf("item 2 sem produto: %+v", b)
	}
	// Nada é gravado na conferência.
	if e.count("suppliers") != 0 || e.count("purchases") != 0 {
		t.Fatal("conferência não pode gravar")
	}

	// Confirma escolhendo o caderno com caixa de 12.
	d, err := svc.ConfirmFile(e.ctx, sampleNFe, []models.NFeDecision{
		{Number: 1, ProductID: caneta.ID, Factor: 1},
		{Number: 2, ProductID: caderno.ID, Factor: 12},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Purchase.AccessKey == nil || *d.Purchase.AccessKey != pv.AccessKey || d.Purchase.InvoiceNumber != "1234" || len(d.Items) != 2 {
		t.Fatalf("compra importada: %+v", d.Purchase)
	}
	// Item 2: 240,00 + frete 8,00 + ST 12,34 − desconto 10,00 = 250,34 por 24 un → 10,43
	cad, _ := e.products.Get(e.ctx, caderno.ID)
	if cad.StockQty != 24 || cad.CostCents != 1043 {
		t.Fatalf("caderno depois da importação: estoque %d custo %d", cad.StockQty, cad.CostCents)
	}
	if _, err := os.Stat(filepath.Join(e.dataDir, "nfe", pv.AccessKey+".xml")); err != nil {
		t.Fatalf("XML original não foi guardado: %v", err)
	}

	// Segunda vez: fornecedor reconhecido, vínculo lembrado, nota bloqueada.
	pv2, _ := svc.PreviewFile(e.ctx, sampleNFe)
	if !pv2.AlreadyImported || pv2.Supplier.ID == 0 || pv2.Items[1].MatchedBy != "vinculo" || pv2.Items[1].Factor != 12 ||
		pv2.Items[1].Units != 24 {
		t.Fatalf("segunda conferência: %+v", pv2)
	}
	if _, err := svc.ConfirmFile(e.ctx, sampleNFe, nil, false); codeOf(err) != CodeDuplicate {
		t.Fatalf("nota duplicada: %v", err)
	}
	if e.count("suppliers") != 1 {
		t.Fatalf("fornecedor duplicado: %d", e.count("suppliers"))
	}
}

func TestNFeImportRules(t *testing.T) {
	e := newEnv(t)
	svc := NewNFeService(NewBase(e.db), e.dataDir)
	p := e.product("Fita", 100, 0)

	// Quantidade fracionada (1,5 UN) não vira unidade inteira: bloqueia.
	frac := nfeVariant(t, "<qCom>10.0000</qCom>", "<qCom>1.5000</qCom>")
	pv, err := svc.PreviewFile(e.ctx, frac)
	if err != nil || pv.Items[0].Units != 0 || !strings.Contains(pv.Items[0].Problem, "inteiro") {
		t.Fatalf("fracionado: %+v %v", pv.Items[0], err)
	}
	_, err = svc.ConfirmFile(e.ctx, frac, []models.NFeDecision{{Number: 1, ProductID: p.ID, Factor: 1}, {Number: 2, ProductID: p.ID, Factor: 1}}, false)
	if codeOf(err) != CodeInvalid || e.count("purchases") != 0 || e.count("suppliers") != 0 {
		t.Fatalf("confirmação com fração deveria falhar sem gravar nada: %v", err)
	}
	// Fator 2 resolve (1,5 × 2 = 3) e os dois itens no mesmo produto são somados.
	d, err := svc.ConfirmFile(e.ctx, frac, []models.NFeDecision{{Number: 1, ProductID: p.ID, Factor: 2}, {Number: 2, ProductID: p.ID, Factor: 1}}, false)
	if err != nil || len(d.Items) != 1 || d.Items[0].Qty != 5 {
		t.Fatalf("itens somados: %+v %v", d.Items, err)
	}

	// Frete fora do custo, configurado.
	st, _ := NewSettingsService(NewBase(e.db)).Get(e.ctx)
	st.NFeCostFreight = false
	if _, err := NewSettingsService(NewBase(e.db)).Save(e.ctx, st); err != nil {
		t.Fatal(err)
	}
	other := nfeVariant(t, "35261011222333000181550010000012341000012345", "35261011222333000181550010000099991000099999")
	pv, _ = svc.PreviewFile(e.ctx, other)
	if pv.Items[0].CostCents != 2600 { // 25,00 + IPI 1,00, sem frete
		t.Fatalf("custo sem frete: %d", pv.Items[0].CostCents)
	}

	if _, err := svc.PreviewFile(e.ctx, filepath.Join(t.TempDir(), "nao-existe.xml")); codeOf(err) != CodeInvalid {
		t.Fatalf("arquivo inexistente: %v", err)
	}
	op := e.user("caixa", models.RoleOperator)
	if _, err := svc.PreviewFile(e.as(op), sampleNFe); codeOf(err) != CodeAuthRequired {
		t.Fatalf("operador sem compras: %v", err)
	}
}

func TestCancelPurchaseAndReimport(t *testing.T) {
	e := newEnv(t).withCash()
	svc := NewNFeService(NewBase(e.db), e.dataDir)
	ps := NewPurchaseService(NewBase(e.db))
	ean := "7891234567895"
	cost := int64(200)
	caneta, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Caneta", Barcode: &ean, PriceCents: 500, Active: true,
		InitialStock: 5, CostCents: &cost})
	caderno := e.product("Caderno", 2500, 0)
	decisions := []models.NFeDecision{{Number: 1, ProductID: caneta.ID, Factor: 1}, {Number: 2, ProductID: caderno.ID, Factor: 12}}

	d, err := svc.ConfirmFile(e.ctx, sampleNFe, decisions, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Cancel(e.ctx, d.Purchase.ID, ""); codeOf(err) != CodeInvalid {
		t.Fatalf("estorno sem motivo: %v", err)
	}

	// Vendeu tudo do caderno: o estorno é recusado e nada muda.
	if _, err := e.sell(models.PaymentCash, nil, item(caderno.ID, 24)); err != nil {
		t.Fatal(err)
	}
	if _, err := ps.Cancel(e.ctx, d.Purchase.ID, "lançada errada"); codeOf(err) != CodeInsufficientStock || e.stockOf(caneta.ID) != 15 {
		t.Fatalf("estorno com mercadoria já vendida: %v (caneta %d)", err, e.stockOf(caneta.ID))
	}
	if _, err := e.stock.RegisterEntry(e.ctx, caderno.ID, 24, "reposição"); err != nil {
		t.Fatal(err)
	}

	got, err := ps.Cancel(e.ctx, d.Purchase.ID, "lançada errada")
	if err != nil {
		t.Fatal(err)
	}
	if got.Purchase.CancelledAt == nil || got.Purchase.AccessKey != nil || got.Purchase.CancelledAccessKey == nil {
		t.Fatalf("compra estornada: %+v", got.Purchase)
	}
	c, _ := e.products.Get(e.ctx, caneta.ID)
	if c.StockQty != 5 || c.CostCents != 200 {
		t.Fatalf("caneta deveria voltar a estoque 5 e custo 2,00: %d / %d", c.StockQty, c.CostCents)
	}
	if _, err := ps.Cancel(e.ctx, d.Purchase.ID, "de novo"); codeOf(err) != CodeStateConflict {
		t.Fatalf("estorno duplo: %v", err)
	}

	// A mesma nota pode ser importada de novo.
	pv, _ := svc.PreviewFile(e.ctx, sampleNFe)
	if pv.AlreadyImported {
		t.Fatal("depois do estorno a nota deveria ficar livre")
	}
	if _, err := svc.ConfirmFile(e.ctx, sampleNFe, decisions, false); err != nil {
		t.Fatalf("reimportar: %v", err)
	}
	if list, _ := ps.List(e.ctx, today(), today(), "35261011222333000181550010000012341000012345"); len(list) != 2 {
		t.Fatalf("busca pela chave deveria achar a estornada e a nova: %d", len(list))
	}
}

func TestNFeCreateAndRenameOnConfirm(t *testing.T) {
	e := newEnv(t)
	svc := NewNFeService(NewBase(e.db), e.dataDir)
	ean := "7891234567895"
	caneta, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Caneta", Barcode: &ean, PriceCents: 500, Active: true})

	// Produto novo sem preço: recusa e não grava nada.
	_, err := svc.ConfirmFile(e.ctx, sampleNFe, []models.NFeDecision{
		{Number: 1, ProductID: caneta.ID, Factor: 1},
		{Number: 2, Factor: 12, Name: "Caderno 10 matérias"},
	}, false)
	if codeOf(err) != CodeInvalid || e.count("purchases") != 0 || len(must(e.products.List(e.ctx))) != 1 {
		t.Fatalf("produto novo sem preço deveria falhar sem gravar: %v", err)
	}

	price := int64(3290)
	d, err := svc.ConfirmFile(e.ctx, sampleNFe, []models.NFeDecision{
		{Number: 1, ProductID: caneta.ID, Factor: 1, Name: "CANETA AZUL BIC", Code: "7891234567895"},
		{Number: 2, Factor: 12, Name: "CADERNO 10 MATÉRIAS", Code: "2000000000017", NewPriceCents: &price},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := e.products.Get(e.ctx, caneta.ID); c.Name != "CANETA AZUL BIC" || c.PriceCents != 500 {
		t.Fatalf("caneta renomeada: %+v", c)
	}
	novo, err := e.products.SearchByBarcode(e.ctx, "2000000000017")
	if err != nil || novo.Name != "CADERNO 10 MATÉRIAS" || novo.PriceCents != 3290 || novo.StockQty != 24 {
		t.Fatalf("produto cadastrado na confirmação: %+v %v", novo, err)
	}
	if len(d.Items) != 2 {
		t.Fatalf("itens: %d", len(d.Items))
	}
}

func must[T any](v T, _ error) T { return v }

func TestNFePriceLater(t *testing.T) {
	e := newEnv(t).withCash()
	svc := NewNFeService(NewBase(e.db), e.dataDir)
	decisions := []models.NFeDecision{
		{Number: 1, Factor: 1, Name: "Caneta azul", Code: "7891234567895"},
		{Number: 2, Factor: 12, Name: "Caderno"},
	}
	if _, err := svc.ConfirmFile(e.ctx, sampleNFe, decisions, false); codeOf(err) != CodeInvalid {
		t.Fatalf("sem preço e sem 'precificar depois' deveria recusar: %v", err)
	}
	if _, err := svc.ConfirmFile(e.ctx, sampleNFe, decisions, true); err != nil {
		t.Fatal(err)
	}
	p, err := e.products.SearchByBarcode(e.ctx, "7891234567895")
	if err != nil || !p.PricePending || p.PriceCents != 0 || p.StockQty != 10 {
		t.Fatalf("produto a precificar: %+v %v", p, err)
	}
	// Fora da venda: não aparece na busca de venda, não vende e não entra em comanda.
	if list, _ := e.products.Search(e.ctx, "caneta", true, 0); len(list) != 0 {
		t.Fatal("produto a precificar não pode aparecer na busca de venda")
	}
	if list, _ := e.products.Search(e.ctx, "caneta", false, 0); len(list) != 1 {
		t.Fatal("na busca geral (compras/produtos) ele aparece")
	}
	if _, err := e.sell(models.PaymentCash, nil, item(p.ID, 1)); codeOf(err) != CodeProductInactive {
		t.Fatalf("venda de produto a precificar: %v", err)
	}
	c, _ := e.comandas.Open(e.ctx, "MESA 1")
	if _, err := e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 1); codeOf(err) != CodeProductInactive {
		t.Fatalf("comanda com produto a precificar: %v", err)
	}
	// Editar sem mudar o preço mantém a marca; definir o preço tira.
	in := models.ProductInput{ID: p.ID, Name: "Caneta azul BIC", Barcode: p.Barcode, PriceCents: 0, Active: true}
	if got, _ := e.products.Update(e.ctx, in); !got.PricePending {
		t.Fatal("editar o nome não deveria tirar 'a precificar'")
	}
	in.PriceCents = 350
	if got, _ := e.products.Update(e.ctx, in); got.PricePending {
		t.Fatal("definir o preço deveria tirar 'a precificar'")
	}
	if _, err := e.sell(models.PaymentCash, nil, item(p.ID, 1)); err != nil {
		t.Fatalf("depois de precificado vende: %v", err)
	}
}

func TestNFeSkipMissingItem(t *testing.T) {
	e := newEnv(t)
	svc := NewNFeService(NewBase(e.db), e.dataDir)
	caneta := e.product("Caneta", 500, 0)
	caderno := e.product("Caderno", 2500, 0)
	// Os dois itens fora: recusado, nada gravado.
	all := []models.NFeDecision{{Number: 1, ProductID: caneta.ID, Factor: 1, Skip: true}, {Number: 2, ProductID: caderno.ID, Factor: 12, Skip: true}}
	if _, err := svc.ConfirmFile(e.ctx, sampleNFe, all, false); codeOf(err) != CodeInvalid || e.count("purchases") != 0 {
		t.Fatalf("todos os itens fora: %v", err)
	}
	// O caderno não veio: só a caneta entra.
	d, err := svc.ConfirmFile(e.ctx, sampleNFe, []models.NFeDecision{{Number: 1, ProductID: caneta.ID, Factor: 1},
		{Number: 2, ProductID: caderno.ID, Factor: 12, Skip: true}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) != 1 || e.stockOf(caderno.ID) != 0 || e.stockOf(caneta.ID) == 0 || !strings.Contains(d.Purchase.Notes, "não recebidos: 2") {
		t.Fatalf("entrada sem o item que não veio: %+v estoque caderno %d", d.Purchase, e.stockOf(caderno.ID))
	}
}
