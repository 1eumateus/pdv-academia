// Caminho: pdv/internal/service/sale_test.go

package service

import (
	"errors"
	"math"
	"sync"
	"testing"

	"pdv/internal/models"
)

func TestCreateSaleSuccess(t *testing.T) {
	e := newEnv(t).withCash()
	arroz := e.product("Arroz 5 kg", 2590, 10)
	feijao := e.product("Feijão", 899, 5)

	d, err := e.sales.CreateSale(e.ctx, models.CreateSaleInput{
		Items:         []models.SaleItemInput{item(arroz.ID, 2), item(feijao.ID, 3), item(arroz.ID, 1)},
		PaymentMethod: models.PaymentCash, DiscountCents: 77,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 3 × 25,90 + 3 × 8,99 = 77,70 + 26,97 = 104,67; menos 0,77 = 103,90
	if d.Sale.SubtotalCents != 10467 || d.Sale.DiscountCents != 77 || d.Sale.TotalCents != 10390 {
		t.Fatalf("totais incorretos: %+v", d.Sale)
	}
	if len(d.Items) != 2 || d.Items[0].Qty != 3 || d.Items[0].ProductNameSnapshot != "Arroz 5 kg" {
		t.Fatalf("itens não consolidados: %+v", d.Items)
	}
	if e.stockOf(arroz.ID) != 7 || e.stockOf(feijao.ID) != 2 {
		t.Fatal("estoque não baixado")
	}
	if e.count("stock_movements") != 4 { // 2 saldos iniciais + 2 baixas
		t.Fatalf("movimentações = %d", e.count("stock_movements"))
	}
	cur, _ := e.cash.Current(e.ctx)
	if cur.ExpectedCashCents != 10000+10390 {
		t.Fatalf("caixa esperado = %d", cur.ExpectedCashCents)
	}
}

func TestSaleKeepsSnapshotAfterProductChange(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Lápis", 150, 10)
	d, err := e.sell(models.PaymentPix, nil, item(p.ID, 2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.products.Update(e.ctx, models.ProductInput{ID: p.ID, Name: "Lápis HB", PriceCents: 300, Active: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.sales.Details(e.ctx, d.Sale.ID)
	if got.Items[0].ProductNameSnapshot != "Lápis" || got.Items[0].UnitPriceCents != 150 || got.Sale.TotalCents != 300 {
		t.Fatalf("venda antiga mudou: %+v", got)
	}
}

func TestSaleValidation(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Régua", 500, 10)
	cases := []struct {
		name string
		in   models.CreateSaleInput
		code Code
	}{
		{"sem itens", models.CreateSaleInput{PaymentMethod: "dinheiro"}, CodeInvalid},
		{"quantidade zero", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 0)}, PaymentMethod: "dinheiro"}, CodeInvalid},
		{"quantidade negativa", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, -1)}, PaymentMethod: "dinheiro"}, CodeInvalid},
		{"quantidade excessiva", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, MaxQty+1)}, PaymentMethod: "dinheiro"}, CodeInvalid},
		{"forma desconhecida", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "cheque"}, CodeInvalid},
		{"desconto maior", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "dinheiro", DiscountCents: 501}, CodeInvalid},
		{"desconto negativo", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "dinheiro", DiscountCents: -1}, CodeInvalid},
		{"produto inexistente", models.CreateSaleInput{Items: []models.SaleItemInput{item(999, 1)}, PaymentMethod: "dinheiro"}, CodeProductNotFound},
		{"cliente inexistente", models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 1)}, PaymentMethod: "dinheiro", CustomerID: ptr(int64(999))}, CodeCustomerNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := e.sales.CreateSale(e.ctx, c.in)
			wantCode(t, err, c.code)
		})
	}
	if e.count("sales") != 0 || e.stockOf(p.ID) != 10 {
		t.Fatal("venda inválida deixou rastros")
	}
}

// Os limites de entrada mantêm o maior total possível (500 itens × 100 mil
// unidades × R$ 100 milhões ≈ 5×10^17) abaixo de MaxInt64 (≈ 9,2×10^18); as
// funções com verificação de estouro são a segunda barreira.
func TestMoneyOverflowChecks(t *testing.T) {
	worst := int64(MaxItemsInSale) * MaxQty * MaxPriceCents
	if worst <= 0 || worst > math.MaxInt64/10 {
		t.Fatalf("limites permitem estouro: %d", worst)
	}
	if _, err := mulCents(math.MaxInt64/2, 3); !errors.Is(err, ErrInvalid) {
		t.Fatal("estouro na multiplicação não detectado")
	}
	if _, err := addCents(math.MaxInt64, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("estouro na soma não detectado")
	}
	if v, err := mulCents(3, 1285); err != nil || v != 3855 {
		t.Fatalf("multiplicação: %d %v", v, err)
	}
	if FormatBRL(123456789) != "R$ 1.234.567,89" || FormatBRL(5) != "R$ 0,05" || FormatBRL(-1050) != "-R$ 10,50" {
		t.Fatalf("FormatBRL: %s %s %s", FormatBRL(123456789), FormatBRL(5), FormatBRL(-1050))
	}
}

func TestSaleInactiveProduct(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Fora de linha", 100, 5)
	e.products.Update(e.ctx, models.ProductInput{ID: p.ID, Name: p.Name, PriceCents: 100, Active: false})
	_, err := e.sell("dinheiro", nil, item(p.ID, 1))
	wantCode(t, err, CodeProductInactive)
}

func TestSaleRequiresOpenCash(t *testing.T) {
	e := newEnv(t)
	p := e.product("Bala", 10, 5)
	_, err := e.sell("dinheiro", nil, item(p.ID, 1))
	wantCode(t, err, CodeCashNotOpen)
}

func TestInsufficientStockMessage(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Arroz 5 kg", 2590, 2)
	_, err := e.sell("dinheiro", nil, item(p.ID, 3))
	wantCode(t, err, CodeInsufficientStock)
	want := `Estoque insuficiente para o produto "Arroz 5 kg". Disponível: 2; solicitado: 3.`
	if err.Error() != want {
		t.Fatalf("mensagem = %q", err.Error())
	}
	var ise *InsufficientStockError
	if !errors.As(err, &ise) || ise.Available != 2 || ise.Requested != 3 {
		t.Fatalf("detalhes do erro: %+v", ise)
	}
}

func TestSaleRollbackOnFailures(t *testing.T) {
	for _, f := range []struct{ event, table string }{
		{"INSERT", "sale_items"},      // falha na inserção de um item
		{"INSERT", "stock_movements"}, // falha no registro da movimentação
		{"UPDATE", "products"},        // falha na atualização do estoque
		{"INSERT", "cash_movements"},  // falha no efeito financeiro
	} {
		t.Run(f.event+"_"+f.table, func(t *testing.T) {
			e := newEnv(t).withCash()
			a := e.product("A", 100, 5)
			b := e.product("B", 200, 5)
			before := e.snap([]int64{a.ID, b.ID}, nil)
			e.failOn(f.event, f.table)
			if _, err := e.sell("dinheiro", nil, item(a.ID, 1), item(b.ID, 2)); err == nil {
				t.Fatal("venda deveria falhar")
			}
			e.assertUnchanged(before, []int64{a.ID, b.ID}, nil)
		})
	}
}

func TestConcurrentSalesOfLastItem(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Último", 1000, 1)
	const n = 10
	var (
		wg             sync.WaitGroup
		mu             sync.Mutex
		ok, outOfStock int
		other          []error
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.sell("dinheiro", nil, item(p.ID, 1))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, ErrInsufficientStock):
				outOfStock++
			default:
				other = append(other, err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || outOfStock != n-1 || len(other) != 0 {
		t.Fatalf("sucesso=%d sem estoque=%d outros=%v", ok, outOfStock, other)
	}
	if e.stockOf(p.ID) != 0 || e.count("sales") != 1 {
		t.Fatal("estado final inconsistente")
	}
}

func TestRepeatedRequestDoesNotDuplicateSale(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Pão", 50, 100)
	in := models.CreateSaleInput{Items: []models.SaleItemInput{item(p.ID, 2)}, PaymentMethod: "dinheiro", RequestID: "carrinho-1"}

	var wg sync.WaitGroup
	ids := make([]int64, 5)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, err := e.sales.CreateSale(e.ctx, in)
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = d.Sale.ID
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("cliques repetidos geraram vendas diferentes: %v", ids)
		}
	}
	if e.count("sales") != 1 || e.stockOf(p.ID) != 98 {
		t.Fatalf("vendas=%d estoque=%d", e.count("sales"), e.stockOf(p.ID))
	}
}

func TestCancelSale(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Caderno", 1500, 10)
	d, err := e.sell("cartao", nil, item(p.ID, 3))
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.sales.CancelSale(e.ctx, d.Sale.ID, "Cliente desistiu")
	if err != nil {
		t.Fatal(err)
	}
	if c.Sale.Status != models.SaleCancelled || c.Sale.CancelledAt == nil || *c.Sale.CancellationReason != "Cliente desistiu" {
		t.Fatalf("cancelamento não registrado: %+v", c.Sale)
	}
	if e.stockOf(p.ID) != 10 {
		t.Fatal("estoque não revertido")
	}
	if e.count("sale_items") != 1 || e.count("sales") != 1 {
		t.Fatal("histórico original foi apagado")
	}
	cur, _ := e.cash.Current(e.ctx)
	var refund int64
	for _, m := range cur.Movements {
		if m.Type == models.CashSaleRefund {
			refund = m.AmountCents
		}
	}
	if refund != -4500 {
		t.Fatalf("estorno no caixa = %d", refund)
	}

	_, err = e.sales.CancelSale(e.ctx, d.Sale.ID, "de novo")
	wantCode(t, err, CodeSaleCancelled)
	if e.stockOf(p.ID) != 10 {
		t.Fatal("cancelamento duplo alterou estoque")
	}
	if _, err := e.sales.CancelSale(e.ctx, d.Sale.ID+1, "x"); !errors.Is(err, ErrSaleNotFound) {
		t.Fatalf("venda inexistente: %v", err)
	}
	if _, err := e.sales.CancelSale(e.ctx, d.Sale.ID, " "); !errors.Is(err, ErrInvalid) {
		t.Fatal("cancelamento sem motivo aceito")
	}
}

func TestCancelSaleRollback(t *testing.T) {
	e := newEnv(t).withCash()
	p := e.product("Cola", 300, 10)
	d, _ := e.sell("dinheiro", nil, item(p.ID, 2))
	before := e.snap([]int64{p.ID}, nil)
	e.failOn("INSERT", "cash_movements")
	if _, err := e.sales.CancelSale(e.ctx, d.Sale.ID, "teste"); err == nil {
		t.Fatal("cancelamento deveria falhar")
	}
	e.assertUnchanged(before, []int64{p.ID}, nil)
	got, _ := e.sales.Details(e.ctx, d.Sale.ID)
	if got.Sale.Status != models.SaleCompleted {
		t.Fatal("status mudou apesar do rollback")
	}
}
