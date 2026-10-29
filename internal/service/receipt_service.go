// Caminho: pdv/internal/service/receipt_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/printer"
	"pdv/internal/repository"
)

// ReceiptService monta comprovantes NÃO FISCAIS como linhas neutras. A tela
// desenha essas linhas em HTML e a impressora térmica as recebe em ESC/POS,
// então o conteúdo é o mesmo nos dois caminhos.
type ReceiptService struct {
	Base
	settings  *SettingsService
	sales     *SaleService
	cashSvc   *CashService
	cash      repository.CashRepo
	credits   repository.CreditRepo
	customers repository.CustomerRepo
	// Send é a saída para a impressora; trocada nos testes.
	Send func(printerName string, data []byte) error
}

func NewReceiptService(b Base) *ReceiptService {
	return &ReceiptService{Base: b, settings: NewSettingsService(b), sales: NewSaleService(b),
		cashSvc: NewCashService(b), Send: printer.Send}
}

var methodLabels = map[string]string{
	models.PaymentCash: "Dinheiro", models.PaymentPix: "Pix", models.PaymentCard: "Cartão", models.PaymentDebit: "Débito",
	models.PaymentCreditCard: "Crédito", models.PaymentCredit: "A prazo", models.PaymentStoreCredit: "Vale-troca",
	models.PaymentMixed: "Misto",
}

// brl formata centavos como "R$ 1.234,56" (negativos com sinal antes do R$).
func brl(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	reais := fmt.Sprint(cents / 100)
	for i := len(reais) - 3; i > 0; i -= 3 {
		reais = reais[:i] + "." + reais[i:]
	}
	return fmt.Sprintf("%sR$ %s,%02d", sign, reais, cents%100)
}

// num: valor como em brl, sem o "R$ " (colunas do item).
func num(cents int64) string { return strings.Replace(brl(cents), "R$ ", "", 1) }

func brDateTime(t time.Time) string { return t.Local().Format("02/01/2006 15:04") }

// formatDocument pontua CPF (000.000.000-00) e CNPJ (00.000.000/0000-00).
func formatDocument(d string) string {
	switch len(d) {
	case 11:
		return "CPF " + d[:3] + "." + d[3:6] + "." + d[6:9] + "-" + d[9:]
	case 14:
		return "CNPJ " + d[:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:]
	}
	return d
}

// addressLines segue o mesmo formato de formatAddress no frontend.
func addressLines(street, number, complement, district, city, state, zip string) []string {
	join := func(sep string, parts ...string) string {
		var keep []string
		for _, p := range parts {
			if p != "" {
				keep = append(keep, p)
			}
		}
		return strings.Join(keep, sep)
	}
	line1 := join(", ", street, number)
	if complement != "" {
		line1 = join(" - ", line1, complement)
	}
	if zip != "" && len(zip) == 8 {
		zip = "CEP " + zip[:5] + "-" + zip[5:]
	}
	var out []string
	for _, l := range []string{line1, join(" - ", district, join("/", city, state)), zip} {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// builder acumula as linhas do comprovante.
type builder struct{ lines []models.ReceiptLine }

func (b *builder) center(text string, bold bool) {
	if text != "" {
		b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineText, Text: text, Center: true, Bold: bold})
	}
}
func (b *builder) text(text string) {
	if text != "" {
		b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineText, Text: text})
	}
}
func (b *builder) pair(left, right string, bold bool) {
	b.lines = append(b.lines, models.ReceiptLine{Kind: models.LinePair, Text: left, Right: right, Bold: bold})
}
// itemHeader e item: produto, quantidade, preço e total numa linha só (valores
// sem "R$" para caber; o cabeçalho diz o que é cada coluna).
func (b *builder) itemHeader() {
	b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineItem, Text: "Produto", Qty: "Qtd", Price: "Preço", Right: "Total", Bold: true})
}
func (b *builder) item(name string, qty int64, priceCents *int64, totalCents int64) {
	price := ""
	if priceCents != nil {
		price = num(*priceCents)
	}
	b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineItem, Text: name, Qty: fmt.Sprint(qty), Price: price, Right: num(totalCents)})
}
func (b *builder) sep() { b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineSeparator}) }
func (b *builder) signature(label string) {
	b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineSignature, Text: label})
}

func (b *builder) header(st models.Settings, title string) {
	b.center(st.StoreName, true)
	if st.LegalName != st.StoreName {
		b.center(st.LegalName, false)
	}
	doc := formatDocument(st.Document)
	if st.StateRegistration != "" {
		doc = strings.TrimSpace(doc + "  IE " + st.StateRegistration)
	}
	b.center(doc, false)
	for _, l := range addressLines(st.Street, st.Number, st.Complement, st.District, st.City, st.State, st.ZipCode) {
		b.center(l, false)
	}
	var contact []string
	if st.Phone != "" {
		contact = append(contact, "Tel. "+st.Phone)
	}
	if st.Whatsapp != "" {
		contact = append(contact, "WhatsApp "+st.Whatsapp)
	}
	b.center(strings.Join(contact, "  "), false)
	b.center(st.Email, false)
	b.sep()
	b.center(title, true)
	b.sep()
}

func (b *builder) footer(st models.Settings) {
	b.sep()
	b.center(st.ReceiptFooter, false)
	b.center("DOCUMENTO NÃO FISCAL", true)
}

func (b *builder) customer(c models.Customer) {
	b.text("Cliente: " + c.Name)
	if c.Document != nil {
		b.text(formatDocument(*c.Document))
	}
	if c.Phone != nil {
		b.text("Tel. " + *c.Phone)
	}
	for _, l := range addressLines(str(c.Street), str(c.Number), str(c.Complement), str(c.District), str(c.City),
		str(c.State), str(c.ZipCode)) {
		b.text(l)
	}
	if c.Reference != nil {
		b.text("Ref.: " + *c.Reference)
	}
}

// Build monta o comprovante do tipo pedido. id é ignorado no teste de impressão.
func (s *ReceiptService) Build(ctx context.Context, kind string, id int64) (models.Receipt, error) {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return models.Receipt{}, err
	}
	var b builder
	title := ""
	switch kind {
	case models.ReceiptSale:
		title, err = s.sale(ctx, &b, st, id)
	case models.ReceiptCreditPayment:
		title, err = s.creditPayment(ctx, &b, st, id)
	case models.ReceiptCashSession:
		title, err = s.cashSession(ctx, &b, st, id)
	case models.ReceiptCashMovement:
		title, err = s.cashMovement(ctx, &b, st, id)
	case models.ReceiptQuote:
		title, err = s.quote(ctx, &b, st, id)
	case models.ReceiptReport:
		title, err = s.report(ctx, &b, st, id)
	case models.ReceiptReturn:
		title, err = s.returnReceipt(ctx, &b, st, id)
	case models.ReceiptTest:
		title = "TESTE DE IMPRESSÃO"
		b.header(st, title)
		b.text("Se você está lendo isto, a impressora está funcionando.")
		b.pair("Acentuação", "ÁÉÍÓÚ ÃÕ Ç", false)
		b.pair("Valor de exemplo", brl(123456), true)
		b.text(brDateTime(s.Now()))
	default:
		return models.Receipt{}, invalid("Tipo de comprovante desconhecido: %s.", kind)
	}
	if err != nil {
		return models.Receipt{}, err
	}
	b.footer(st)
	return models.Receipt{Title: title, Logo: st.Logo, Lines: b.lines}, nil
}

func (s *ReceiptService) sale(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	d, err := s.sales.Details(ctx, id)
	if err != nil {
		return "", err
	}
	sale := d.Sale
	title := "COMPROVANTE DE VENDA"
	b.header(st, title)
	b.pair(fmt.Sprintf("Venda nº %d", sale.ID), brDateTime(sale.CreatedAt), false)
	if sale.ComandaID != nil {
		b.text(fmt.Sprintf("Comanda nº %d", *sale.ComandaID))
	}
	if sale.UserName != nil {
		b.text("Operador: " + *sale.UserName)
	}
	if sale.Status == models.SaleCancelled {
		b.center("*** VENDA CANCELADA ***", true)
		if sale.CancelledByName != nil {
			b.text("Cancelada por: " + *sale.CancelledByName)
		}
	}
	b.sep()
	b.itemHeader()
	for _, it := range d.Items {
		b.item(it.ProductNameSnapshot, it.Qty, &it.UnitPriceCents, it.SubtotalCents)
		if it.PromoLabel != "" && it.PromoDiscountCents == 0 {
			b.text("  " + it.PromoLabel)
		}
		if it.PromoDiscountCents > 0 {
			b.pair("  "+it.PromoLabel, brl(-it.PromoDiscountCents), false)
		}
		if it.DiscountCents > 0 {
			b.pair("  Desconto no item", brl(-it.DiscountCents), false)
		}
	}
	b.sep()
	b.pair("Subtotal", brl(sale.SubtotalCents), false)
	if sale.DiscountCents > 0 {
		b.pair("Desconto", brl(-sale.DiscountCents), false)
	}
	b.pair("TOTAL", brl(sale.TotalCents), true)
	// Uma linha por forma de pagamento; troco logo abaixo do dinheiro.
	hasFiado := false
	for _, p := range d.Payments {
		label := methodLabels[p.Method]
		if p.CardBrand != "" {
			label += " " + p.CardBrand
		}
		if p.Installments > 1 {
			label += fmt.Sprintf(" %dx", p.Installments)
		}
		b.pair(label, brl(p.AmountCents), false)
		if p.ChangeCents > 0 {
			b.pair("  Recebido", brl(p.ReceivedCents), false)
			b.pair("  Troco", brl(p.ChangeCents), true)
		}
		hasFiado = hasFiado || p.Method == models.PaymentCredit
	}
	if len(d.Payments) == 0 {
		b.pair("Pagamento", methodLabels[sale.PaymentMethod], false)
	}
	// Devoluções desta venda: o que voltou, quanto e como foi reembolsado.
	if len(d.Returns) > 0 {
		b.sep()
		b.center("*** ITENS DEVOLVIDOS ***", true)
		var returned int64
		for _, r := range d.Returns {
			b.pair(fmt.Sprintf("Devolução nº %d", r.ID), brDateTime(r.CreatedAt), false)
			b.itemHeader()
			for _, it := range r.Items {
				b.item(it.ProductName, it.Qty, nil, -it.AmountCents)
			}
			b.pair("  Reembolso: "+refundLabels[r.RefundMethod], brl(-r.TotalCents), false)
			returned += r.TotalCents
		}
		b.pair("TOTAL DEVOLVIDO", brl(-returned), true)
		b.pair("VALOR FINAL DA VENDA", brl(sale.TotalCents-returned), true)
	}
	if d.Customer != nil {
		b.sep()
		b.customer(*d.Customer)
		if hasFiado {
			b.pair("Saldo devedor atual", brl(d.Customer.CreditBalanceCents), true)
			s.pixForFiado(ctx, b, st, d)
			b.signature("Assinatura do cliente")
		}
	}
	// Venda para entrega (já paga): o entregador leva o cupom com o endereço.
	if d.Delivery != nil {
		b.sep()
		b.center("*** PARA ENTREGA ***", true)
		if d.Customer != nil {
			b.text("Para: " + d.Customer.Name)
		}
		for _, l := range strings.Split(d.Delivery.Address, "\n") {
			b.text(strings.TrimSpace(l))
		}
		if d.Delivery.Phone != "" {
			b.text("Tel. " + d.Delivery.Phone)
		}
		if d.Delivery.Notes != "" {
			b.text("Obs.: " + d.Delivery.Notes)
		}
		b.center("VENDA JÁ PAGA", true)
		b.signature("Recebido por")
	}
	return title, nil
}

func (s *ReceiptService) quote(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	d, err := NewQuoteService(s.Base).Get(ctx, id)
	if err != nil {
		return "", err
	}
	q := d.Quote
	title := fmt.Sprintf("ORÇAMENTO Nº %d", q.ID)
	b.header(st, title)
	b.pair("Emitido em", brDateTime(q.CreatedAt), false)
	b.pair("Válido até", strings.Join(reverse(strings.Split(q.ValidUntil, "-")), "/"), true)
	if q.UserName != nil {
		b.text("Operador: " + *q.UserName)
	}
	if d.Customer != nil {
		b.sep()
		b.customer(*d.Customer)
	}
	b.sep()
	b.itemHeader()
	for _, it := range d.Items {
		b.item(it.ProductName, it.Qty, &it.UnitPriceCents, it.Qty*it.UnitPriceCents)
		if it.PromoDiscountCents > 0 {
			b.pair("  "+it.PromoLabel, brl(-it.PromoDiscountCents), false)
		} else if it.PromoLabel != "" {
			b.text("  " + it.PromoLabel)
		}
		if it.DiscountCents > 0 {
			b.pair("  Desconto no item", brl(-it.DiscountCents), false)
		}
	}
	b.sep()
	b.pair("Subtotal", brl(q.SubtotalCents), false)
	if q.DiscountCents > 0 {
		b.pair("Desconto", brl(-q.DiscountCents), false)
	}
	b.pair("TOTAL", brl(q.TotalCents), true)
	if q.Notes != "" {
		b.sep()
		b.text("Obs.: " + q.Notes)
	}
	b.sep()
	b.center("ORÇAMENTO — NÃO É DOCUMENTO FISCAL", true)
	b.center("Preços e estoque sujeitos a alteração após a validade.", false)
	return title, nil
}

func (s *ReceiptService) creditPayment(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	p, err := s.credits.GetByID(ctx, s.DB, id)
	if err != nil {
		return "", fromRepo(err, newErr(CodeNotFound, "Recebimento %d não encontrado.", id), "")
	}
	c, err := s.customers.GetByID(ctx, s.DB, p.CustomerID)
	if err != nil {
		return "", err
	}
	title := "RECIBO DE PAGAMENTO - FIADO"
	if p.Kind == models.CreditPaymentReversal {
		title = "ESTORNO DE PAGAMENTO - FIADO"
	}
	b.header(st, title)
	b.pair(fmt.Sprintf("Recibo nº %d", p.ID), brDateTime(p.CreatedAt), false)
	if name := s.userOf(ctx, "credit_payments", p.ID); name != "" {
		b.text("Operador: " + name)
	}
	if p.Reversed {
		b.center("*** PAGAMENTO ESTORNADO ***", true)
	}
	b.sep()
	b.customer(c)
	b.sep()
	b.pair("Valor", brl(p.AmountCents), true)
	b.pair("Forma de pagamento", methodLabels[p.PaymentMethod], false)
	if ids, err := s.credits.AllocatedSales(ctx, s.DB, p.ID); err != nil {
		return "", err
	} else if len(ids) > 0 {
		nums := make([]string, len(ids))
		for i, id := range ids {
			nums[i] = fmt.Sprintf("nº %d", id)
		}
		b.text("Notas pagas: " + strings.Join(nums, ", "))
	}
	if p.Notes != nil {
		b.text("Obs.: " + *p.Notes)
	}
	b.pair("Saldo devedor atual", brl(c.CreditBalanceCents), true)
	b.signature("Recebido por")
	return title, nil
}

func (s *ReceiptService) cashSession(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	sum, err := s.cashSvc.Summary(ctx, id)
	if err != nil {
		return "", err
	}
	ses := sum.Session
	title := "FECHAMENTO DE CAIXA"
	if ses.Status == models.CashOpen {
		title = "CAIXA ABERTO - RESUMO PARCIAL"
	}
	b.header(st, title)
	b.text(fmt.Sprintf("Caixa nº %d", ses.ID))
	b.pair("Abertura", brDateTime(ses.OpenedAt), false)
	if ses.OpenedByName != nil {
		b.pair("Aberto por", *ses.OpenedByName, false)
	}
	if ses.ClosedAt != nil {
		b.pair("Fechamento", brDateTime(*ses.ClosedAt), false)
	}
	if ses.ClosedByName != nil {
		b.pair("Fechado por", *ses.ClosedByName, false)
	}
	b.sep()
	b.pair("Saldo inicial", brl(ses.OpeningCents), false)
	var sales int64
	for _, v := range sum.Sales {
		if v.Status != models.SaleCancelled {
			sales++
		}
	}
	b.pair("Vendas concluídas", fmt.Sprint(sales), false)
	if len(sum.ByPaymentMethod) > 0 {
		b.sep()
		b.text("Movimentação por forma:")
		for _, t := range sum.ByPaymentMethod {
			b.pair("  "+methodLabels[t.PaymentMethod], brl(t.TotalCents), false)
		}
	}
	b.sep()
	b.pair("Esperado em dinheiro", brl(sum.ExpectedCashCents), true)
	if ses.CountedCents != nil {
		b.pair("Contado", brl(*ses.CountedCents), false)
	}
	if ses.DifferenceCents != nil {
		b.pair("Diferença", brl(*ses.DifferenceCents), true)
	}
	if ses.Notes != nil {
		b.text("Obs.: " + *ses.Notes)
	}
	b.signature("Responsável pelo caixa")
	return title, nil
}

func (s *ReceiptService) cashMovement(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	m, err := s.cash.GetMovement(ctx, s.DB, id)
	if err != nil {
		return "", fromRepo(err, newErr(CodeNotFound, "Movimentação %d não encontrada.", id), "")
	}
	var title string
	switch m.Type {
	case models.CashWithdrawal:
		title = "SANGRIA DE CAIXA"
	case models.CashSupply:
		title = "SUPRIMENTO DE CAIXA"
	default:
		return "", invalid("Só sangria e suprimento têm comprovante próprio.")
	}
	b.header(st, title)
	b.pair(fmt.Sprintf("Movimento nº %d", m.ID), brDateTime(m.CreatedAt), false)
	b.text(fmt.Sprintf("Caixa nº %d", m.SessionID))
	if name := s.userOf(ctx, "cash_movements", m.ID); name != "" {
		b.text("Operador: " + name)
	}
	b.sep()
	amount := m.AmountCents
	if amount < 0 {
		amount = -amount
	}
	b.pair("Valor", brl(amount), true)
	if m.Description != nil {
		b.text("Motivo: " + *m.Description)
	}
	b.signature("Responsável")
	return title, nil
}

func (s *ReceiptService) report(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	v, r, err := NewReportService(s.Base).Saved(ctx, id)
	if err != nil {
		return "", err
	}
	br := func(d string) string { return strings.Join(reverse(strings.Split(d, "-")), "/") }
	title := strings.ToUpper(reportNames[v.Kind])
	show := func(kind string) bool { return v.Kind == models.ReportGeneral || v.Kind == kind }
	b.header(st, title)
	b.pair(fmt.Sprintf("Relatório nº %d", v.ID), brDateTime(v.CreatedAt), false)
	b.pair("Período", br(r.StartDate)+" a "+br(r.EndDate), true)
	if v.FilterUserName != nil {
		b.text("Operador: " + *v.FilterUserName)
	}
	if v.UserName != nil {
		b.text("Gerado por: " + *v.UserName)
	}
	b.sep()
	b.pair("Vendas concluídas", fmt.Sprint(r.SaleCount), false)
	b.pair("Descontos", brl(r.DiscountCents), false)
	b.pair(fmt.Sprintf("Canceladas (%d)", r.CancelledCount), brl(r.CancelledCents), false)
	b.pair("Recebido a prazo", brl(r.CreditReceivedCents), false)
	b.pair("TOTAL VENDIDO", brl(r.TotalCents), true)
	if show(models.ReportByPayment) && len(r.ByPaymentMethod) > 0 {
		b.sep()
		b.text("Por forma de pagamento:")
		for _, t := range r.ByPaymentMethod {
			b.pair(fmt.Sprintf("  %s (%d)", methodLabels[t.PaymentMethod], t.Count), brl(t.TotalCents), false)
		}
	}
	if show(models.ReportByCategory) && len(r.ByCategory) > 0 {
		b.sep()
		b.text("Por categoria:")
		for _, t := range r.ByCategory {
			b.pair(fmt.Sprintf("  %s (%d)", t.CategoryName, t.Qty), brl(t.TotalCents), false)
		}
	}
	if show(models.ReportByOperator) && len(r.ByOperator) > 0 {
		b.sep()
		b.text("Por operador:")
		for _, t := range r.ByOperator {
			b.pair(fmt.Sprintf("  %s (%d)", t.UserName, t.Count), brl(t.TotalCents), false)
		}
	}
	if show("") && len(r.Products) > 0 {
		b.sep()
		b.text("Produtos vendidos:")
		b.itemHeader()
		for _, p := range r.Products {
			b.item(p.Name, p.Qty, nil, p.TotalCents)
		}
	}
	return title, nil
}

var refundLabels = map[string]string{
	models.RefundStoreCredit: "Vale-troca", models.RefundCash: "Dinheiro", models.RefundCredit: "Abatido no a prazo",
	models.RefundPix: "Pix", models.RefundCard: "Estorno no cartão",
}

func (s *ReceiptService) returnReceipt(ctx context.Context, b *builder, st models.Settings, id int64) (string, error) {
	r, err := (repository.ReturnRepo{}).GetByID(ctx, s.DB, id)
	if err != nil {
		return "", fromRepo(err, newErr(CodeNotFound, "Devolução %d não encontrada.", id), "")
	}
	title := "COMPROVANTE DE DEVOLUÇÃO"
	b.header(st, title)
	b.pair(fmt.Sprintf("Devolução nº %d", r.ID), brDateTime(r.CreatedAt), false)
	b.text(fmt.Sprintf("Venda de origem nº %d", r.SaleID))
	if r.UserName != nil {
		b.text("Operador: " + *r.UserName)
	}
	b.sep()
	b.itemHeader()
	for _, it := range r.Items {
		b.item(it.ProductName, it.Qty, nil, it.AmountCents)
	}
	b.sep()
	b.pair("TOTAL DEVOLVIDO", brl(r.TotalCents), true)
	b.pair("Reembolso", refundLabels[r.RefundMethod], false)
	b.text("Motivo: " + r.Reason)
	if r.CustomerID != nil {
		c, err := s.customers.GetByID(ctx, s.DB, *r.CustomerID)
		if err != nil {
			return "", err
		}
		b.sep()
		b.customer(c)
		switch r.RefundMethod {
		case models.RefundStoreCredit:
			b.pair("Saldo de vale-troca", brl(c.StoreCreditCents), true)
		case models.RefundCredit:
			b.pair("Saldo devedor atual", brl(c.CreditBalanceCents), true)
		}
	}
	b.signature("Assinatura do cliente")
	return title, nil
}

// userOf devolve o nome de quem gravou o registro (vazio se antigo/sem usuário).
// table vem sempre de constante no código, nunca do usuário.
func (s *ReceiptService) userOf(ctx context.Context, table string, id int64) string {
	var name sql.NullString
	_ = s.DB.QueryRowContext(ctx, `SELECT u.name FROM `+table+` t LEFT JOIN users u ON u.id = t.user_id WHERE t.id = ?`, id).Scan(&name)
	return name.String
}

// Print monta o comprovante e envia direto para a impressora térmica
// configurada. Falha de impressão nunca desfaz a operação já gravada.
func (s *ReceiptService) Print(ctx context.Context, kind string, id int64) error {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	if st.PaperWidth == "a4" || st.PrinterName == "" {
		return invalid("Nenhuma impressora térmica configurada. Escolha a impressora em Configurações.")
	}
	r, err := s.Build(ctx, kind, id)
	if err != nil {
		return err
	}
	return s.send(st, r, st.OpenDrawer)
}

// send manda o comprovante para a térmica configurada.
func (s *ReceiptService) send(st models.Settings, r models.Receipt, openDrawer bool) error {
	data := printer.Render(r, printer.Options{Width: st.PaperWidth, Cut: st.CutPaper, OpenDrawer: openDrawer})
	if err := s.Send(st.PrinterName, data); err != nil {
		return newErr(CodeInvalid, "Não foi possível imprimir em \"%s\": %v. Confira se a impressora está ligada, com papel, "+
			"e tente reimprimir. A operação já foi registrada normalmente.", st.PrinterName, err)
	}
	return nil
}

// PrintPix imprime na térmica o QR Code Pix de um valor, para o cliente ler
// com o celular (sem abrir a gaveta).
func (s *ReceiptService) PrintPix(ctx context.Context, amountCents int64, txid string) error {
	st, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	if st.PaperWidth == "a4" || st.PrinterName == "" {
		return invalid("Nenhuma impressora térmica configurada. Escolha a impressora em Configurações.")
	}
	code, err := s.settings.PixQR(ctx, amountCents, txid)
	if err != nil {
		return err
	}
	var b builder
	b.header(st, "PAGAMENTO VIA PIX")
	b.pair("Valor", brl(amountCents), true)
	b.text(brDateTime(s.Now()))
	b.sep()
	b.center("Leia o QR Code no app do seu banco", false)
	b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineQR, Text: code.Payload})
	b.footer(st)
	return s.send(st, models.Receipt{Title: "PAGAMENTO VIA PIX", Logo: st.Logo, Lines: b.lines}, false)
}

// pixForFiado acrescenta o QR Pix do valor em fiado, se ligado em Configurações.
// Sem chave ou com erro, o cupom sai normalmente sem o QR.
func (s *ReceiptService) pixForFiado(ctx context.Context, b *builder, st models.Settings, d models.SaleDetails) {
	if !st.PixOnReceipt || st.PixKey == "" {
		return
	}
	var fiado int64
	for _, p := range d.Payments {
		if p.Method == models.PaymentCredit {
			fiado += p.AmountCents
		}
	}
	code, err := s.settings.PixQR(ctx, fiado, fmt.Sprintf("FIADO%d", d.Sale.ID))
	if err != nil || fiado == 0 {
		return
	}
	b.center("Pague "+brl(fiado)+" com Pix:", true)
	b.lines = append(b.lines, models.ReceiptLine{Kind: models.LineQR, Text: code.Payload, Right: code.Image})
}
