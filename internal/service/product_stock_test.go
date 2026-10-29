// Caminho: pdv/internal/service/product_stock_test.go

package service

import (
	"errors"
	"testing"

	"pdv/internal/models"
)

func TestProductCreateAndUpdate(t *testing.T) {
	e := newEnv(t)
	code := " 789000 "
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "  Arroz 5 kg ", Barcode: &code, PriceCents: 2590, Active: true, InitialStock: 10})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Arroz 5 kg" || *p.Barcode != "789000" || p.StockQty != 10 {
		t.Fatalf("produto gravado incorreto: %+v", p)
	}
	// O saldo inicial vira movimentação.
	if e.count("stock_movements") != 1 {
		t.Fatal("saldo inicial não registrado como movimentação")
	}
	p2, err := e.products.Update(e.ctx, models.ProductInput{ID: p.ID, Name: "Arroz Tipo 1", Barcode: p.Barcode, PriceCents: 2790, Active: false})
	if err != nil {
		t.Fatal(err)
	}
	if p2.Name != "Arroz Tipo 1" || p2.PriceCents != 2790 || p2.Active || p2.StockQty != 10 {
		t.Fatalf("edição incorreta: %+v", p2)
	}
	found, err := e.products.SearchByBarcode(e.ctx, "789000")
	if err != nil || found.ID != p.ID {
		t.Fatalf("busca por código falhou: %v", err)
	}
	list, err := e.products.Search(e.ctx, "tipo", false, 0)
	if err != nil || len(list) != 1 {
		t.Fatalf("busca por nome falhou: %v %d", err, len(list))
	}
	if list, _ := e.products.Search(e.ctx, "tipo", true, 0); len(list) != 0 {
		t.Fatal("busca de ativos retornou produto inativo")
	}
}

func TestProductValidation(t *testing.T) {
	e := newEnv(t)
	cases := []models.ProductInput{
		{Name: "", PriceCents: 100},
		{Name: "X", PriceCents: -1},
		{Name: "X", PriceCents: 100, InitialStock: -1},
	}
	for _, c := range cases {
		if _, err := e.products.Create(e.ctx, c); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: esperado erro de validação, veio %v", c, err)
		}
	}
}

func TestDuplicateBarcode(t *testing.T) {
	e := newEnv(t)
	code := "123"
	if _, err := e.products.Create(e.ctx, models.ProductInput{Name: "A", Barcode: &code, PriceCents: 1, Active: true}); err != nil {
		t.Fatal(err)
	}
	_, err := e.products.Create(e.ctx, models.ProductInput{Name: "B", Barcode: &code, PriceCents: 1, Active: true})
	wantCode(t, err, CodeDuplicate)
	if e.count("products") != 1 {
		t.Fatal("produto duplicado foi gravado")
	}
	// Código vazio não conta como duplicado.
	empty := "  "
	for i := 0; i < 2; i++ {
		if _, err := e.products.Create(e.ctx, models.ProductInput{Name: "Sem código", Barcode: &empty, PriceCents: 1, Active: true}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStockEntryAndAdjust(t *testing.T) {
	e := newEnv(t)
	p := e.product("Caneta", 250, 0)

	if _, err := e.stock.RegisterEntry(e.ctx, p.ID, 20, "Nota 123"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.stock.Adjust(e.ctx, p.ID, 5, "Contagem"); err != nil {
		t.Fatal(err)
	}
	got, err := e.stock.Adjust(e.ctx, p.ID, -3, "Avaria")
	if err != nil {
		t.Fatal(err)
	}
	if got.StockQty != 22 || e.stockOf(p.ID) != 22 {
		t.Fatalf("saldo = %d, esperado 22", got.StockQty)
	}
	if e.count("stock_movements") != 3 {
		t.Fatalf("movimentações = %d, esperado 3", e.count("stock_movements"))
	}
	if _, err := e.stock.Adjust(e.ctx, p.ID, -1, ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("ajuste sem motivo foi aceito")
	}
	if _, err := e.stock.RegisterEntry(e.ctx, p.ID, 0, ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("entrada zero foi aceita")
	}
	if audit, _ := e.stock.Audit(e.ctx); len(audit) != 0 {
		t.Fatalf("auditoria acusou divergência indevida: %+v", audit)
	}
}

func TestStockCannotGoNegative(t *testing.T) {
	e := newEnv(t)
	p := e.product("Borracha", 100, 2)
	before := e.snap([]int64{p.ID}, nil)
	_, err := e.stock.Adjust(e.ctx, p.ID, -3, "Perda")
	if !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("esperado estoque insuficiente, veio %v", err)
	}
	e.assertUnchanged(before, []int64{p.ID}, nil)
}

func TestStockAuditAndReconcile(t *testing.T) {
	e := newEnv(t)
	p := e.product("Caderno", 1500, 10)
	// Alteração feita por fora do sistema.
	if _, err := e.db.Exec(`UPDATE products SET stock_qty = 7 WHERE id = ?`, p.ID); err != nil {
		t.Fatal(err)
	}
	audit, err := e.stock.Audit(e.ctx)
	if err != nil || len(audit) != 1 {
		t.Fatalf("auditoria: %v %+v", err, audit)
	}
	if a := audit[0]; a.StoredQty != 7 || a.CalculatedQty != 10 || a.DifferenceQty != -3 {
		t.Fatalf("divergência incorreta: %+v", a)
	}
	if _, err := e.stock.ReconcileDivergence(e.ctx, p.ID, "Ajuste manual no banco"); err != nil {
		t.Fatal(err)
	}
	if audit, _ := e.stock.Audit(e.ctx); len(audit) != 0 {
		t.Fatalf("divergência persiste: %+v", audit)
	}
	if e.count("stock_movements") != 2 || e.stockOf(p.ID) != 7 {
		t.Fatal("reconciliação deveria só acrescentar uma movimentação e manter o saldo")
	}
	if _, err := e.stock.ReconcileDivergence(e.ctx, p.ID, "de novo"); !errors.Is(err, ErrStateConflict) {
		t.Fatalf("reconciliar sem divergência deveria falhar, veio %v", err)
	}
}

func TestNoStockProductSellsWithoutStock(t *testing.T) {
	e := newEnv(t).withCash()
	p, err := e.products.Create(e.ctx, models.ProductInput{Name: "Plano mensal", PriceCents: 9000, Active: true, NoStock: true})
	if err != nil || !p.NoStock {
		t.Fatalf("cadastro sem estoque: %v %+v", err, p)
	}
	before := e.count("stock_movements")
	if _, err := e.sell("dinheiro", nil, item(p.ID, 3)); err != nil {
		t.Fatalf("produto sem controle de estoque deveria vender sem saldo: %v", err)
	}
	if e.stockOf(p.ID) != 0 || e.count("stock_movements") != before {
		t.Fatal("produto sem controle de estoque não pode mexer no saldo nem gerar movimentação")
	}
	if _, err := e.stock.RegisterEntry(e.ctx, p.ID, 5, ""); codeOf(err) != CodeInvalid {
		t.Fatalf("entrada em produto sem estoque: esperava OPERACAO_INVALIDA, veio %v", err)
	}
	// Editar o produto sem mandar NoStock volta a controlar (o formulário sempre manda).
	in := models.ProductInput{ID: p.ID, Name: p.Name, PriceCents: p.PriceCents, Active: true, NoStock: true}
	if p, _ = e.products.Update(e.ctx, in); !p.NoStock {
		t.Fatal("editar mantendo NoStock não pode ligar o controle")
	}
}

func TestComandaCannotExceedStock(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Cerveja", 1000, 3)
	c, err := e.comandas.Open(e.ctx, "Mesa 1")
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.comandas.AddItem(e.ctx, c.Comanda.ID, p.ID, 2); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("2 + 2 com estoque 3: esperava estoque insuficiente, veio %v", err)
	}
	if _, err := e.comandas.SetItemQty(e.ctx, d.Items[0].ID, 4); !errors.Is(err, ErrInsufficientStock) {
		t.Fatalf("quantidade 4 com estoque 3: esperava estoque insuficiente, veio %v", err)
	}
	if _, err := e.comandas.SetItemQty(e.ctx, d.Items[0].ID, 3); err != nil {
		t.Fatalf("quantidade igual ao estoque deveria passar: %v", err)
	}
}

func TestStockMovementsValueAndCostHidden(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Água", 350, 10)
	if _, err := e.sell("dinheiro", nil, item(p.ID, 2)); err != nil {
		t.Fatal(err)
	}
	ms, err := e.stock.ListMovements(e.ctx, 0, today(), today())
	if err != nil {
		t.Fatal(err)
	}
	var sale, initial *models.StockMovement
	for i := range ms {
		if ms[i].Reason == "venda" {
			sale = &ms[i]
		} else {
			initial = &ms[i]
		}
	}
	if sale == nil || sale.UnitCents == nil || *sale.UnitCents != 350 || sale.IsCost || sale.UserName == nil {
		t.Fatalf("venda deveria vir com o preço e o operador: %+v", sale)
	}
	if initial == nil || !initial.IsCost || initial.UnitCents == nil {
		t.Fatalf("saldo inicial deveria vir com o custo para o admin: %+v", initial)
	}
	// Caixa sem permissão de ver custo: o custo some, o preço da venda fica.
	ms, _ = e.stock.ListMovements(e.as(e.user("caixa1", models.RoleOperator)), 0, today(), today())
	for _, m := range ms {
		if m.IsCost && m.UnitCents != nil || !m.IsCost && m.UnitCents == nil {
			t.Fatalf("custo vazou ou preço sumiu: %+v", m)
		}
	}
}
