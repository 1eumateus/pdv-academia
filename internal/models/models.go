// Caminho: pdv/internal/models/models.go

// Package models define as estruturas de domínio e os DTOs trocados com o
// frontend. Valores monetários são int64 em centavos; datas são time.Time
// (serializadas pelo Wails como texto RFC 3339).
package models

import "time"

// Formas de pagamento. 'pix' e 'cartao' são apenas registros feitos pelo
// operador: não há integração com instituição financeira.
const (
	PaymentCash        = "dinheiro"
	PaymentPix         = "pix"
	PaymentCard        = "cartao" // só em registros antigos; hoje é débito ou crédito
	PaymentDebit       = "debito"
	PaymentCreditCard  = "credito"
	PaymentCredit      = "fiado"
	PaymentStoreCredit = "vale_troca"
	PaymentMixed       = "misto" // resumo da venda paga em mais de uma forma
)

// PaymentInput é uma parte do pagamento. ReceivedCents só vale para dinheiro
// (troco = recebido − valor); bandeira e parcelas são só informativas.
type PaymentInput struct {
	Method        string `json:"method"`
	AmountCents   int64  `json:"amountCents"`
	ReceivedCents int64  `json:"receivedCents"`
	CardBrand     string `json:"cardBrand"`
	Installments  int64  `json:"installments"`
}

type SalePayment struct {
	ID            int64  `json:"id"`
	Method        string `json:"method"`
	AmountCents   int64  `json:"amountCents"`
	ReceivedCents int64  `json:"receivedCents"`
	ChangeCents   int64  `json:"changeCents"`
	CardBrand     string `json:"cardBrand"`
	Installments  int64  `json:"installments"`
}

// Status de venda.
const (
	SaleCompleted = "concluida"
	SaleCancelled = "cancelada"
)

// Status de comanda.
const (
	ComandaOpen      = "aberta"
	ComandaClosed    = "fechada"
	ComandaDiscarded = "descartada"
)

// Motivos de movimentação de estoque.
const (
	StockEntry        = "entrada"
	StockSale         = "venda"
	StockAdjust       = "ajuste"
	StockReturn       = "devolucao"
	StockCancellation = "cancelamento"
)

// Status de sessão de caixa.
const (
	CashOpen   = "aberta"
	CashClosed = "fechada"
)

// Tipos de movimentação de caixa.
const (
	CashSupply          = "suprimento"
	CashWithdrawal      = "sangria"
	CashSale            = "venda"
	CashCreditPayment   = "recebimento_fiado"
	CashSaleRefund      = "estorno_venda"
	CashPaymentReversal = "estorno_recebimento"
	CashReturn          = "devolucao" // reembolso em dinheiro de uma devolução
)

// Tipos de recebimento de crédito.
const (
	CreditPaymentReceipt  = "recebimento"
	CreditPaymentReversal = "estorno"
)

type Product struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Barcode    *string   `json:"barcode"`
	Sku        *string   `json:"sku"` // código interno opcional
	PriceCents int64     `json:"priceCents"`
	StockQty   int64     `json:"stockQty"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`

	CategoryID   *int64  `json:"categoryId"`
	CategoryName *string `json:"categoryName"`
	// Variação de grade: modelo, tamanho e cor (vazios em produto comum).
	ModelID   *int64  `json:"modelId"`
	ModelName *string `json:"modelName"`
	Size      string  `json:"size"`
	Color     string  `json:"color"`
	// CostCents vem zerado para quem não tem a permissão ver_custo.
	CostCents int64 `json:"costCents"`
	MinStock  int64 `json:"minStock"`
	// PricePending: "a precificar" — não pode ser vendido até ter preço definido.
	PricePending bool `json:"pricePending"`
	// NoStock: não controla estoque (vende sem saldo, não dá baixa).
	NoStock bool `json:"noStock"`
	// PlanDays: produto é um plano de assinatura com esta duração (nil = produto comum).
	PlanDays *int64 `json:"planDays"`
	// Description: texto livre (serviços e planos).
	Description *string `json:"description"`
	// Promo: promoção vigente hoje (só na busca de venda; o backend recalcula ao vender).
	Promo *ProductPromo `json:"promo"`
}

// Supplier é usado no cadastro (ID 0 = novo) e nas listagens. Document guarda
// só os caracteres do CPF/CNPJ.
type Supplier struct {
	ID                int64  `json:"id"`
	LegalName         string `json:"legalName"`
	TradeName         string `json:"tradeName"`
	Document          string `json:"document"`
	StateRegistration string `json:"stateRegistration"`
	Contact           string `json:"contact"`
	Phone             string `json:"phone"`
	Email             string `json:"email"`
	ZipCode           string `json:"zipCode"`
	Street            string `json:"street"`
	Number            string `json:"number"`
	Complement        string `json:"complement"`
	District          string `json:"district"`
	City              string `json:"city"`
	State             string `json:"state"`
	Notes             string `json:"notes"`
	Active            bool   `json:"active"`
}

type Purchase struct {
	ID            int64     `json:"id"`
	SupplierID    int64     `json:"supplierId"`
	SupplierName  string    `json:"supplierName"`
	InvoiceNumber string    `json:"invoiceNumber"`
	IssueDate     string    `json:"issueDate"` // AAAA-MM-DD
	AccessKey     *string   `json:"accessKey"` // chave da NF-e, quando importada do XML
	ProductsCents int64     `json:"productsCents"`
	FreightCents  int64     `json:"freightCents"`
	TaxesCents    int64     `json:"taxesCents"`
	DiscountCents int64     `json:"discountCents"`
	TotalCents    int64     `json:"totalCents"`
	Notes         string    `json:"notes"`
	UserName      *string   `json:"userName"`
	CreatedAt     time.Time `json:"createdAt"`
	// Estorno: estoque devolvido e custo restaurado; a chave da nota fica livre para reimportar.
	CancelledAt        *time.Time `json:"cancelledAt"`
	CancelledByName    *string    `json:"cancelledByName"`
	CancellationReason *string    `json:"cancellationReason"`
	CancelledAccessKey *string    `json:"cancelledAccessKey"`
}

type PurchaseItem struct {
	ID              int64  `json:"id"`
	ProductID       int64  `json:"productId"`
	ProductName     string `json:"productName"`
	Qty             int64  `json:"qty"`
	UnitCostCents   int64  `json:"unitCostCents"`
	TotalCents      int64  `json:"totalCents"`
	CostBeforeCents int64  `json:"costBeforeCents"`
	CostAfterCents  int64  `json:"costAfterCents"`
}

type PurchaseDetails struct {
	Purchase Purchase       `json:"purchase"`
	Items    []PurchaseItem `json:"items"`
}

// PurchaseItemInput: NewPriceCents opcional ajusta o preço de venda na mesma operação.
type PurchaseItemInput struct {
	ProductID     int64  `json:"productId"`
	Qty           int64  `json:"qty"`
	UnitCostCents int64  `json:"unitCostCents"`
	NewPriceCents *int64 `json:"newPriceCents"`
}

type PurchaseInput struct {
	SupplierID    int64               `json:"supplierId"`
	InvoiceNumber string              `json:"invoiceNumber"`
	IssueDate     string              `json:"issueDate"`
	Notes         string              `json:"notes"`
	Items         []PurchaseItemInput `json:"items"`
	// Preenchidos pela importação do XML (T8); na entrada manual ficam zerados.
	AccessKey     string `json:"accessKey"`
	FreightCents  int64  `json:"freightCents"`
	TaxesCents    int64  `json:"taxesCents"`
	DiscountCents int64  `json:"discountCents"`
}

// NFeItem é um item da nota já conferido: produto vinculado (se houver),
// quantidade convertida em unidades e custo unitário final.
type NFeItem struct {
	Number    int    `json:"number"`
	Code      string `json:"code"`
	EAN       string `json:"ean"`
	Name      string `json:"name"`
	NCM       string `json:"ncm"`
	Unit      string `json:"unit"`
	QtyText   string `json:"qtyText"` // quantidade da nota, ex.: "2" ou "1,5"
	LineCents int64  `json:"lineCents"`
	// Custo do item na nota: produtos + frete/IPI/ST/outros − desconto, conforme Configurações.
	CostCents     int64   `json:"costCents"`
	ProductID     *int64  `json:"productId"`
	ProductName   *string `json:"productName"`
	ProductCode   *string `json:"productCode"` // código de barras do produto do sistema
	MatchedBy     string  `json:"matchedBy"`   // "codigo_barras", "vinculo" ou "" (sem produto)
	Factor        int64   `json:"factor"`
	Units         int64   `json:"units"` // 0 se a quantidade convertida não for inteira
	UnitCostCents int64   `json:"unitCostCents"`
	PriceCents    int64   `json:"priceCents"`
	CurrentCost   int64   `json:"currentCostCents"` // zerado sem ver_custo
	Problem       string  `json:"problem"`
}

type NFePreview struct {
	AccessKey       string    `json:"accessKey"`
	Number          string    `json:"number"`
	Series          string    `json:"series"`
	IssueDate       string    `json:"issueDate"`
	Supplier        Supplier  `json:"supplier"` // ID 0 = será cadastrado com os dados da nota
	AlreadyImported bool      `json:"alreadyImported"`
	InvoiceCents    int64     `json:"invoiceCents"`
	Items           []NFeItem `json:"items"`
}

// NFeDecision é o que o usuário escolheu para um item na conferência. Com
// ProductID 0 e Name preenchido, o produto é cadastrado na confirmação (com
// NewPriceCents como preço); com ProductID, Name/Code diferentes renomeiam o produto.
type NFeDecision struct {
	Number        int    `json:"number"`
	ProductID     int64  `json:"productId"`
	Factor        int64  `json:"factor"`
	NewPriceCents *int64 `json:"newPriceCents"`
	Name          string `json:"name"`
	Code          string `json:"code"`
	// Skip: o item não veio na entrega; fica fora da entrada (estoque e custo).
	Skip bool `json:"skip"`
}

// ProductModel agrupa as variações de uma grade.
type ProductModel struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	CategoryID     *int64   `json:"categoryId"`
	BasePriceCents int64    `json:"basePriceCents"`
	Sizes          []string `json:"sizes"`
	Colors         []string `json:"colors"`
	VariationCount int64    `json:"variationCount"`
}

// GridInput cria uma grade nova (ID 0) ou acrescenta tamanhos/cores a uma existente.
type GridInput struct {
	ModelID        int64    `json:"modelId"`
	Name           string   `json:"name"`
	CategoryID     *int64   `json:"categoryId"`
	BasePriceCents int64    `json:"basePriceCents"`
	Sizes          []string `json:"sizes"`
	Colors         []string `json:"colors"`
}

type Category struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ProductInput é usado no cadastro e na edição. O estoque inicial só é aceito
// no cadastro e vira uma movimentação de entrada.
type ProductInput struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Barcode      *string `json:"barcode"`
	Sku          *string `json:"sku"`
	PriceCents   int64   `json:"priceCents"`
	Active       bool    `json:"active"`
	InitialStock int64   `json:"initialStock"`
	CategoryID   *int64  `json:"categoryId"`
	// CostCents nil mantém o custo atual (quem não vê o custo não o altera).
	CostCents *int64 `json:"costCents"`
	MinStock  int64  `json:"minStock"`
	// AutoSku/AutoBarcode: no cadastro, grava o próximo código interno livre
	// (o que o formulário mostrou pode ter sido usado por outro caixa).
	AutoSku     bool `json:"autoSku"`
	AutoBarcode bool `json:"autoBarcode"`
	// NoStock: não controla estoque (falso, o padrão, controla).
	NoStock bool `json:"noStock"`
	// PlanDays: plano de assinatura com esta duração em dias (nil = produto comum).
	PlanDays *int64 `json:"planDays"`
	// Description: texto livre; nil mantém o atual na edição.
	Description *string `json:"description"`
}

// Subscription é uma assinatura (plano de academia) com a fatura em aberto, se houver.
type Subscription struct {
	ID           int64  `json:"id"`
	CustomerID   int64  `json:"customerId"`
	CustomerName string `json:"customerName"`
	// CustomerPhone: para os lembretes pelo WhatsApp.
	CustomerPhone *string `json:"customerPhone"`
	ProductID     int64   `json:"productId"`
	PlanName      string  `json:"planName"`
	PlanDays      int64   `json:"planDays"`
	// PriceCents: valor especial deste aluno (nil = preço do plano).
	PriceCents *int64 `json:"priceCents"`
	// Rules: exceção deste aluno (nil = segue as regras do plano).
	Rules       *AccessRules `json:"rules"`
	Status      string       `json:"status"` // ativa, cancelada
	CreatedAt   time.Time    `json:"createdAt"`
	CancelledAt *time.Time   `json:"cancelledAt"`
	// PausedAt: pausada desde (nil = correndo); os dias pausados não contam.
	PausedAt    *time.Time `json:"pausedAt"`
	PauseReason *string    `json:"pauseReason"`
	// Fatura aberta (nil se não há); Overdue = vencimento antes de hoje (nunca enquanto pausada).
	OpenInvoice *SubscriptionInvoice `json:"openInvoice"`
	Overdue     bool                 `json:"overdue"`
	LastPaidAt  *time.Time           `json:"lastPaidAt"`
}

type SubscriptionInvoice struct {
	ID          int64      `json:"id"`
	DueDate     string     `json:"dueDate"` // AAAA-MM-DD
	AmountCents int64      `json:"amountCents"`
	Status      string     `json:"status"`      // aberta, paga, nao_paga (antiga, importada), cancelada
	Imported    bool       `json:"imported"`    // veio de outro sistema (sem venda no caixa)
	OutsideCash bool       `json:"outsideCash"` // paga fora do caixa (lançamento retroativo)
	SaleID      *int64     `json:"saleId"`
	PaidAt      *time.Time `json:"paidAt"`
}

// CashReport é o "Controle do caixa": entradas e saídas do período.
type CashReport struct {
	StartDate string            `json:"startDate"`
	EndDate   string            `json:"endDate"`
	UserName  *string           `json:"userName"` // nil = todos os operadores
	Rows      []CashReportRow   `json:"rows"`
	Totals    []CashReportTotal `json:"totals"`
	InCents   int64             `json:"inCents"`
	OutCents  int64             `json:"outCents"`
}

type CashReportRow struct {
	Receipt     string    `json:"receipt"` // nº da venda ("-" sem venda)
	At          time.Time `json:"at"`
	History     string    `json:"history"`
	Detail      string    `json:"detail"` // itens da venda (vazio nos demais)
	AmountCents int64     `json:"amountCents"`
	Method      string    `json:"method"`
	In          bool      `json:"in"` // E (true) ou S
}

type CashReportTotal struct {
	Method   string `json:"method"`
	InCents  int64  `json:"inCents"`
	OutCents int64  `json:"outCents"`
}

// SubscriptionImport: prévia ou resultado da importação de assinaturas por planilha.
type SubscriptionImport struct {
	Applied           bool                     `json:"applied"`
	CustomersNew      int                      `json:"customersNew"`
	CustomersExisting int                      `json:"customersExisting"` // já cadastrados (mesmo CPF, ou nome e nascimento)
	CustomersWithSub  int                      `json:"customersWithSub"`  // já tinham assinatura: pulados
	CustomersSkipped  int                      `json:"customersSkipped"`
	Skipped           []string                 `json:"skipped"` // motivos (até 30)
	Plans             []SubscriptionImportPlan `json:"plans"`
	SubsActive        int                      `json:"subsActive"`
	SubsCancelled     int                      `json:"subsCancelled"`
	InvoicesPaid      int                      `json:"invoicesPaid"`
	InvoicesOpen      int                      `json:"invoicesOpen"`
	InvoicesUnpaidOld int                      `json:"invoicesUnpaidOld"` // meses sem pagamento: ficam no histórico como "não utilizou" (sem cobrança)
	InvoicesSkipped   int                      `json:"invoicesSkipped"`   // de código de cliente que não entrou
	OtherLines        int                      `json:"otherLines"`        // mensalidades com dado inválido: não importadas
	FixedWords        int                      `json:"fixedWords"`        // palavras com "�" corrigidas
}

type SubscriptionImportPlan struct {
	Name       string `json:"name"`
	PriceCents int64  `json:"priceCents"`
	Days       int64  `json:"days"`
	Invoices   int    `json:"invoices"`
	Exists     bool   `json:"exists"`
}

// Bill é uma conta a pagar (boleto, aluguel, energia…).
type Bill struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Payee       string  `json:"payee"`
	AmountCents int64   `json:"amountCents"`
	DueDate     string  `json:"dueDate"` // AAAA-MM-DD
	Barcode     string  `json:"barcode"` // linha digitável / código de barras
	Notes       string  `json:"notes"`
	Recurring   bool    `json:"recurring"` // repete todo mês (gera a próxima ao pagar)
	Status      string  `json:"status"`    // aberta, paga
	PaidDate    *string `json:"paidDate"`
	PaidCents   *int64  `json:"paidCents"`
	PayMethod   *string `json:"payMethod"`
	FromCash    bool    `json:"fromCash"` // pago com dinheiro do caixa (sangria)
	// Quantos arquivos anexados de cada tipo.
	BoletoFiles  int64     `json:"boletoFiles"`
	ReceiptFiles int64     `json:"receiptFiles"`
	CreatedAt    time.Time `json:"createdAt"`
}

type BillInput struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Payee       string `json:"payee"`
	AmountCents int64  `json:"amountCents"`
	DueDate     string `json:"dueDate"`
	Barcode     string `json:"barcode"`
	Notes       string `json:"notes"`
	Recurring   bool   `json:"recurring"`
}

type PayBillInput struct {
	BillID    int64  `json:"billId"`
	PaidDate  string `json:"paidDate"` // AAAA-MM-DD
	PaidCents int64  `json:"paidCents"`
	Method    string `json:"method"`
	FromCash  bool   `json:"fromCash"`
}

type BillFile struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"` // boleto, comprovante
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"createdAt"`
}

type BillCategoryTotal struct {
	Category   string `json:"category"`
	Count      int64  `json:"count"`
	TotalCents int64  `json:"totalCents"`
}

// PayInvoiceInput: pagamento da fatura no caixa (sem a prazo nem vale-troca).
type PayInvoiceInput struct {
	InvoiceID     int64          `json:"invoiceId"`
	PaymentMethod string         `json:"paymentMethod"`
	Payments      []PaymentInput `json:"payments"`
	RequestID     string         `json:"requestId"`
	// NextDue: próximo vencimento escolhido (AAAA-MM-DD; vazio = pagamento + duração do plano).
	NextDue string `json:"nextDue"`
}

// ProductCode é um par SKU (só números) + EAN-13 interno da mesma sequência.
type ProductCode struct {
	Sku     string `json:"sku"`
	Barcode string `json:"barcode"`
}

type Customer struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Phone *string `json:"phone"`
	// Dados opcionais. Documento guarda só dígitos (CPF 11 ou CNPJ 14).
	Document           *string `json:"document"`
	Email              *string `json:"email"`
	ZipCode            *string `json:"zipCode"`
	Street             *string `json:"street"`
	Number             *string `json:"number"`
	Complement         *string `json:"complement"`
	District           *string `json:"district"`
	City               *string `json:"city"`
	State              *string `json:"state"`
	Reference          *string `json:"reference"`
	Notes              *string `json:"notes"`
	CreditLimitCents   int64   `json:"creditLimitCents"`
	CreditBalanceCents int64   `json:"creditBalanceCents"`
	StoreCreditCents   int64   `json:"storeCreditCents"` // vale-troca disponível
	Active             bool    `json:"active"`
	// LastChargeAt: última cobrança do a prazo pelo WhatsApp.
	LastChargeAt *time.Time `json:"lastChargeAt"`
	// BirthDate: AAAA-MM-DD (opcional).
	BirthDate *string   `json:"birthDate"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Como a devolução é reembolsada.
const (
	RefundStoreCredit = "vale_troca" // saldo de vale-troca do cliente
	RefundCash        = "dinheiro"   // sai do caixa
	RefundCredit      = "fiado"      // abate a dívida do cliente
	RefundPix         = "pix"        // a loja devolve por Pix
	RefundCard        = "cartao"     // estorno na maquininha (só venda com cartão)
)

// ReturnableItem é um item da venda com o que ainda pode ser devolvido.
type ReturnableItem struct {
	SaleItemID  int64  `json:"saleItemId"`
	ProductName string `json:"productName"`
	Qty         int64  `json:"qty"`         // vendido
	ReturnedQty int64  `json:"returnedQty"` // já devolvido
	PaidCents   int64  `json:"paidCents"`   // quanto o cliente pagou pelo item inteiro (com descontos)
}

// ReturnInfo alimenta a tela de devolução de uma venda.
type ReturnInfo struct {
	Sale     Sale             `json:"sale"`
	Items    []ReturnableItem `json:"items"`
	Customer *Customer        `json:"customer"`
	HasFiado bool             `json:"hasFiado"` // a venda teve parte em fiado
	// CardLeftCents: quanto ainda pode ser estornado no cartão (0 = venda sem cartão).
	CardLeftCents int64    `json:"cardLeftCents"`
	Methods       []string `json:"methods"` // formas usadas no pagamento da venda
	Returns       []Return `json:"returns"` // devoluções já feitas
}

type ReturnItemInput struct {
	SaleItemID int64 `json:"saleItemId"`
	Qty        int64 `json:"qty"`
}

type ReturnInput struct {
	SaleID       int64             `json:"saleId"`
	Items        []ReturnItemInput `json:"items"`
	RefundMethod string            `json:"refundMethod"`
	// CustomerID: para o vale-troca de uma venda feita sem cliente.
	CustomerID *int64 `json:"customerId"`
	Reason     string `json:"reason"`
}

type ReturnItem struct {
	SaleItemID  int64  `json:"saleItemId"`
	ProductName string `json:"productName"`
	Qty         int64  `json:"qty"`
	AmountCents int64  `json:"amountCents"`
}

type Return struct {
	ID           int64        `json:"id"`
	SaleID       int64        `json:"saleId"`
	CustomerID   *int64       `json:"customerId"`
	CustomerName *string      `json:"customerName"`
	RefundMethod string       `json:"refundMethod"`
	TotalCents   int64        `json:"totalCents"`
	Reason       string       `json:"reason"`
	UserName     *string      `json:"userName"`
	CreatedAt    time.Time    `json:"createdAt"`
	Items        []ReturnItem `json:"items"`
}

type CustomerInput struct {
	ID    int64   `json:"id"`
	Name  string  `json:"name"`
	Phone *string `json:"phone"`
	// Dados opcionais. Documento guarda só dígitos (CPF 11 ou CNPJ 14).
	Document         *string `json:"document"`
	Email            *string `json:"email"`
	ZipCode          *string `json:"zipCode"`
	Street           *string `json:"street"`
	Number           *string `json:"number"`
	Complement       *string `json:"complement"`
	District         *string `json:"district"`
	City             *string `json:"city"`
	State            *string `json:"state"`
	Reference        *string `json:"reference"`
	Notes            *string `json:"notes"`
	CreditLimitCents int64   `json:"creditLimitCents"`
	Active           bool    `json:"active"`
	// BirthDate: AAAA-MM-DD; nil mantém a atual, "" apaga.
	BirthDate *string `json:"birthDate"`
}

type Sale struct {
	ID                 int64      `json:"id"`
	CustomerID         *int64     `json:"customerId"`
	CustomerName       *string    `json:"customerName"`
	ComandaID          *int64     `json:"comandaId"`
	CashSessionID      *int64     `json:"cashSessionId"`
	SubtotalCents      int64      `json:"subtotalCents"`
	DiscountCents      int64      `json:"discountCents"`
	TotalCents         int64      `json:"totalCents"`
	PaymentMethod      string     `json:"paymentMethod"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"createdAt"`
	CancelledAt        *time.Time `json:"cancelledAt"`
	CancellationReason *string    `json:"cancellationReason"`
	// Operador que registrou e quem cancelou (nulos em vendas antigas).
	UserID          *int64  `json:"userId"`
	UserName        *string `json:"userName"`
	CancelledByName *string `json:"cancelledByName"`
	// Resumo dos itens para listas: "3x Coca-Cola 2L + 2 itens".
	ItemsLabel string `json:"itemsLabel"`
}

type SaleItem struct {
	ID                  int64  `json:"id"`
	SaleID              int64  `json:"saleId"`
	ProductID           int64  `json:"productId"`
	ProductNameSnapshot string `json:"productNameSnapshot"`
	Qty                 int64  `json:"qty"`
	UnitPriceCents      int64  `json:"unitPriceCents"`
	SubtotalCents       int64  `json:"subtotalCents"` // qtd × preço
	DiscountCents       int64  `json:"discountCents"` // desconto deste item
	// Promoção aplicada: preço promocional (já no preço unitário) ou "leve X pague Y".
	PromotionID        *int64 `json:"promotionId"`
	PromoDiscountCents int64  `json:"promoDiscountCents"`
	PromoLabel         string `json:"promoLabel"`
	// UnitCostCents: custo do produto na hora da venda (nunca vai para a tela).
	UnitCostCents int64 `json:"-"`
}

// Situações do orçamento ("expirado" é calculado pela validade).
const (
	QuoteOpen      = "aberto"
	QuoteApproved  = "aprovado"
	QuoteConverted = "convertido"
	QuoteCancelled = "cancelado"
	QuoteExpired   = "expirado"
)

type Quote struct {
	ID            int64     `json:"id"`
	CustomerID    *int64    `json:"customerId"`
	CustomerName  *string   `json:"customerName"`
	Status        string    `json:"status"` // inclui "expirado"
	ValidUntil    string    `json:"validUntil"`
	Notes         string    `json:"notes"`
	SubtotalCents int64     `json:"subtotalCents"`
	DiscountCents int64     `json:"discountCents"`
	TotalCents    int64     `json:"totalCents"`
	SaleID        *int64    `json:"saleId"`
	UserName      *string   `json:"userName"`
	CreatedAt     time.Time `json:"createdAt"`
}

type QuoteItem struct {
	ID                 int64  `json:"id"`
	ProductID          int64  `json:"productId"`
	ProductName        string `json:"productName"`
	Barcode            string `json:"barcode"` // código atual do produto (impressão A4)
	Qty                int64  `json:"qty"`
	UnitPriceCents     int64  `json:"unitPriceCents"`
	DiscountCents      int64  `json:"discountCents"`
	PromoDiscountCents int64  `json:"promoDiscountCents"`
	PromoLabel         string `json:"promoLabel"`
}

type QuoteDetails struct {
	Quote    Quote       `json:"quote"`
	Items    []QuoteItem `json:"items"`
	Customer *Customer   `json:"customer"`
}

// QuoteInput: ID 0 = novo. Os preços são sempre os do sistema no momento.
type QuoteInput struct {
	ID            int64           `json:"id"`
	CustomerID    *int64          `json:"customerId"`
	Items         []SaleItemInput `json:"items"`
	DiscountCents int64           `json:"discountCents"`
	Notes         string          `json:"notes"`
	ValidUntil    string          `json:"validUntil"` // vazio = validade padrão das Configurações
}

// QuoteDifference avisa o que mudou entre o orçamento e a venda de agora.
type QuoteDifference struct {
	ProductName string `json:"productName"`
	Message     string `json:"message"`
	Blocking    bool   `json:"blocking"` // impede a conversão (sem estoque, inativo...)
}

// QuoteCheck: o que mudou e o total que a venda teria agora.
type QuoteCheck struct {
	Differences []QuoteDifference `json:"differences"`
	TotalCents  int64             `json:"totalCents"`
}

// QuoteConvertInput: pagamento da venda gerada pelo orçamento.
type QuoteConvertInput struct {
	QuoteID       int64          `json:"quoteId"`
	PaymentMethod string         `json:"paymentMethod"`
	Payments      []PaymentInput `json:"payments"`
	CustomerID    *int64         `json:"customerId"`
	RequestID     string         `json:"requestId"`
}

// Tipos de promoção.
const (
	PromoPrice   = "preco"
	PromoPercent = "percentual"
	PromoBuyPay  = "leve_pague"
)

type Promotion struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	ProductID    *int64  `json:"productId"`
	ProductName  *string `json:"productName"`
	CategoryID   *int64  `json:"categoryId"`
	CategoryName *string `json:"categoryName"`
	PriceCents   int64   `json:"priceCents"`
	Percent      int64   `json:"percent"`
	BuyQty       int64   `json:"buyQty"`
	PayQty       int64   `json:"payQty"`
	StartDate    string  `json:"startDate"`
	EndDate      string  `json:"endDate"`
	Active       bool    `json:"active"`
	Status       string  `json:"status"` // vigente, futura, encerrada, inativa
}

// ProductPromo é a promoção vigente mostrada na busca de venda.
type ProductPromo struct {
	Label      string `json:"label"`
	PriceCents int64  `json:"priceCents"` // preço promocional (0 = só leve X pague Y)
	BuyQty     int64  `json:"buyQty"`
	PayQty     int64  `json:"payQty"`
}

type SaleDetails struct {
	Sale  Sale       `json:"sale"`
	Items []SaleItem `json:"items"`
	// Payments: as partes do pagamento (uma só nas vendas comuns).
	Payments []SalePayment `json:"payments"`
	// Customer traz telefone e endereço atuais, para o comprovante de entrega.
	Customer *Customer `json:"customer"`
	// Delivery: para onde levar, se a venda foi para entrega.
	Delivery *Delivery `json:"delivery"`
	// Returns: devoluções feitas desta venda (itens e quantidades).
	Returns []Return `json:"returns"`
}

// SaleItemInput é o que o frontend envia: só produto e quantidade. Preço e
// total são sempre calculados no backend.
type SaleItemInput struct {
	ProductID int64 `json:"productId"`
	Qty       int64 `json:"qty"`
	// DiscountCents: desconto neste item (em centavos; a tela converte o %).
	DiscountCents int64 `json:"discountCents"`
}

type CreateSaleInput struct {
	Items         []SaleItemInput `json:"items"`
	PaymentMethod string          `json:"paymentMethod"`
	CustomerID    *int64          `json:"customerId"`
	DiscountCents int64           `json:"discountCents"`
	// Payments: várias formas na mesma venda. Vazio = tudo em PaymentMethod.
	Payments []PaymentInput `json:"payments"`
	// RequestID é gerado pelo frontend uma vez por carrinho. Reenviar o mesmo
	// pedido devolve a venda já registrada.
	RequestID string `json:"requestId"`
	// Delivery: venda para entrega (já paga); nil = retirada no balcão.
	Delivery *Delivery `json:"delivery"`
	// QuoteID: a venda veio de um orçamento levado para a tela de vendas; ao
	// gravar, o orçamento fica "convertido" e ligado a esta venda.
	QuoteID *int64 `json:"quoteId"`
}

type PayCreditInput struct {
	// SaleIDs: notas (vendas no fiado) que este recebimento paga; vazio = as mais antigas.
	SaleIDs       []int64 `json:"saleIds"`
	CustomerID    int64   `json:"customerId"`
	AmountCents   int64   `json:"amountCents"`
	PaymentMethod string  `json:"paymentMethod"`
	Notes         string  `json:"notes"`
	RequestID     string  `json:"requestId"`
}

type StockMovement struct {
	ID            int64     `json:"id"`
	ProductID     int64     `json:"productId"`
	ProductName   string    `json:"productName"`
	QtyChange     int64     `json:"qtyChange"`
	Reason        string    `json:"reason"`
	ReferenceType *string   `json:"referenceType"`
	ReferenceID   *int64    `json:"referenceId"`
	Notes         *string   `json:"notes"`
	UserName      *string   `json:"userName"`
	UnitCents     *int64    `json:"unitCents"` // valor unitário: preço da venda/devolução ou custo (nil sem permissão de ver custo)
	IsCost        bool      `json:"isCost"`    // UnitCents é custo (entrada, ajuste), não preço de venda
	CreatedAt     time.Time `json:"createdAt"`
}

// StockAuditEntry compara o saldo gravado com a soma das movimentações.
type StockAuditEntry struct {
	ProductID     int64  `json:"productId"`
	ProductName   string `json:"productName"`
	StoredQty     int64  `json:"storedQty"`
	CalculatedQty int64  `json:"calculatedQty"`
	DifferenceQty int64  `json:"differenceQty"`
}

type Comanda struct {
	ID         int64      `json:"id"`
	TableNo    string     `json:"tableNo"`
	Status     string     `json:"status"`
	OpenedAt   time.Time  `json:"openedAt"`
	ClosedAt   *time.Time `json:"closedAt"`
	TotalCents int64      `json:"totalCents"`
	ItemCount  int64      `json:"itemCount"`
	SaleID     *int64     `json:"saleId"`
}

type ComandaItem struct {
	ID                  int64  `json:"id"`
	ComandaID           int64  `json:"comandaId"`
	ProductID           int64  `json:"productId"`
	ProductNameSnapshot string `json:"productNameSnapshot"`
	Qty                 int64  `json:"qty"`
	UnitPriceCents      int64  `json:"unitPriceCents"`
	SubtotalCents       int64  `json:"subtotalCents"`
	ProductActive       bool   `json:"productActive"`
}

type CloseComandaInput struct {
	ComandaID     int64  `json:"comandaId"`
	PaymentMethod string `json:"paymentMethod"`
	CustomerID    *int64 `json:"customerId"`
	DiscountCents int64  `json:"discountCents"`
	// Payments: várias formas no fechamento. Vazio = tudo em PaymentMethod.
	Payments []PaymentInput `json:"payments"`
}

type ComandaDetails struct {
	Comanda Comanda       `json:"comanda"`
	Items   []ComandaItem `json:"items"`
}

type CreditPayment struct {
	ID                int64     `json:"id"`
	CustomerID        int64     `json:"customerId"`
	CustomerName      string    `json:"customerName"`
	AmountCents       int64     `json:"amountCents"`
	PaymentMethod     string    `json:"paymentMethod"`
	Kind              string    `json:"kind"`
	ReversesPaymentID *int64    `json:"reversesPaymentId"`
	Reversed          bool      `json:"reversed"`
	CreatedAt         time.Time `json:"createdAt"`
	Notes             *string   `json:"notes"`
}

type CustomerHistory struct {
	Customer Customer        `json:"customer"`
	Sales    []Sale          `json:"sales"`
	Payments []CreditPayment `json:"payments"`
	Notes    []FiadoNote     `json:"notes"` // compras no fiado, da mais antiga para a mais nova
}

// FiadoNote é uma compra no fiado (nota) com o que já foi pago dela.
type FiadoNote struct {
	SaleID      int64     `json:"saleId"` // 0 = saldo anterior sem venda
	CreatedAt   time.Time `json:"createdAt"`
	Description string    `json:"description"` // itens da venda
	AmountCents int64     `json:"amountCents"` // parte da venda no fiado
	PaidCents   int64     `json:"paidCents"`
	OpenCents   int64     `json:"openCents"`
	Status      string    `json:"status"` // aberta, parcial, paga
	DaysOpen    int64     `json:"daysOpen"`
}

type CashSession struct {
	ID              int64      `json:"id"`
	Status          string     `json:"status"`
	OpeningCents    int64      `json:"openingCents"`
	ExpectedCents   *int64     `json:"expectedCents"`
	CountedCents    *int64     `json:"countedCents"`
	DifferenceCents *int64     `json:"differenceCents"`
	Notes           *string    `json:"notes"`
	OpenedAt        time.Time  `json:"openedAt"`
	ClosedAt        *time.Time `json:"closedAt"`
	OpenedByName    *string    `json:"openedByName"`
	ClosedByName    *string    `json:"closedByName"`
	// OwnerUserID: dono do caixa no modo "caixa por usuário" (nulo = caixa geral).
	OwnerUserID *int64 `json:"ownerUserId"`
}

type CashMovement struct {
	ID            int64     `json:"id"`
	SessionID     int64     `json:"sessionId"`
	Type          string    `json:"type"`
	PaymentMethod string    `json:"paymentMethod"`
	AmountCents   int64     `json:"amountCents"`
	Description   *string   `json:"description"`
	ReferenceType *string   `json:"referenceType"`
	ReferenceID   *int64    `json:"referenceId"`
	CreatedAt     time.Time `json:"createdAt"`
}

// PaymentTotal soma valores por forma de pagamento.
type PaymentTotal struct {
	PaymentMethod string `json:"paymentMethod"`
	Count         int64  `json:"count"`
	TotalCents    int64  `json:"totalCents"`
}

// CashSummary é a visão da sessão atual (ou de uma sessão passada).
type CashSummary struct {
	Session           CashSession    `json:"session"`
	Movements         []CashMovement `json:"movements"`
	Sales             []Sale         `json:"sales"`
	ByPaymentMethod   []PaymentTotal `json:"byPaymentMethod"`
	ExpectedCashCents int64          `json:"expectedCashCents"`
}

// Dashboard é a tela inicial. Vendas seguem o escopo do usuário (só as
// próprias, sem a permissão de ver as de todos); estoque, fiado e orçamentos são da loja.
type Dashboard struct {
	AllSales        bool          `json:"allSales"`
	Today           PeriodTotals  `json:"today"`
	Yesterday       PeriodTotals  `json:"yesterday"` // ontem até a mesma hora
	Month           PeriodTotals  `json:"month"`
	PrevMonth       PeriodTotals  `json:"prevMonth"` // mês passado até o mesmo dia/hora
	Days            []DayTotal    `json:"days"`      // últimos 30 dias, com zeros
	TopProducts     []ProductSold `json:"topProducts"`
	StockAlerts     []StockAlert  `json:"stockAlerts"`
	LowStockCount   int64         `json:"lowStockCount"`
	ZeroStockCount  int64         `json:"zeroStockCount"`
	CreditOpenCents int64         `json:"creditOpenCents"`
	CreditCustomers int64         `json:"creditCustomers"`
	OpenQuotes      int64         `json:"openQuotes"`
	OpenQuotesCents int64         `json:"openQuotesCents"`
	Cash            *CashSession  `json:"cash"`
	CashExpected    int64         `json:"cashExpectedCents"`
	// Contas a pagar abertas que vencem hoje e as vencidas (a tela só mostra com a permissão).
	BillsDueToday int64 `json:"billsDueToday"`
	BillsOverdue  int64 `json:"billsOverdue"`
}

type PeriodTotals struct {
	Count      int64 `json:"count"`
	TotalCents int64 `json:"totalCents"`
	AvgCents   int64 `json:"avgCents"`
}

type DayTotal struct {
	Date       string `json:"date"`
	Count      int64  `json:"count"`
	TotalCents int64  `json:"totalCents"`
}

type StockAlert struct {
	ProductID int64  `json:"productId"`
	Name      string `json:"name"`
	StockQty  int64  `json:"stockQty"`
	MinStock  int64  `json:"minStock"`
}

type ProductSold struct {
	ProductID  int64  `json:"productId"`
	Name       string `json:"name"`
	Qty        int64  `json:"qty"`
	TotalCents int64  `json:"totalCents"`
	// Estoque atual do produto (no momento em que o relatório foi gerado).
	StockQty int64 `json:"stockQty"`
	MinStock int64 `json:"minStock"`
}

type SalesReport struct {
	// ID do relatório gerado (saved_reports); 0 quando só consultado.
	ID                  int64           `json:"id,omitempty"`
	StartDate           string          `json:"startDate"`
	EndDate             string          `json:"endDate"`
	SaleCount           int64           `json:"saleCount"`
	TotalCents          int64           `json:"totalCents"`
	DiscountCents       int64           `json:"discountCents"`
	CancelledCount      int64           `json:"cancelledCount"`
	CancelledCents      int64           `json:"cancelledCents"`
	ByPaymentMethod     []PaymentTotal  `json:"byPaymentMethod"`
	Products            []ProductSold   `json:"products"`
	NotSold             []ProductSold   `json:"notSold"` // ativos sem venda no período
	LowStockDefault     int64           `json:"lowStockDefault"`
	Sales               []Sale          `json:"sales"`
	CreditPayments      []CreditPayment `json:"creditPayments"`
	CreditReceivedCents int64           `json:"creditReceivedCents"`
	StockMovements      []StockMovement `json:"stockMovements"`
	ByOperator          []OperatorTotal `json:"byOperator"`
	ByCategory          []CategoryTotal `json:"byCategory"`
}

// Tipos de relatório gerado: o geral ou só uma seção dele.
const (
	ReportGeneral    = "geral"
	ReportByPayment  = "pagamento"
	ReportByCategory = "categoria"
	ReportByOperator = "operador"
)

// SavedReport é a linha da lista de relatórios gerados (sem a cópia completa).
type SavedReport struct {
	ID             int64     `json:"id"`
	Kind           string    `json:"kind"` // ReportGeneral, ReportByPayment...
	StartDate      string    `json:"startDate"`
	EndDate        string    `json:"endDate"`
	FilterUserName *string   `json:"filterUserName"`
	SaleCount      int64     `json:"saleCount"`
	TotalCents     int64     `json:"totalCents"`
	UserID         *int64    `json:"-"`
	UserName       *string   `json:"userName"`
	CreatedAt      time.Time `json:"createdAt"`
}

// Backgrounds: imagens de fundo (data URL JPEG/PNG; vazio = fundo padrão).
// Ficam fora de Settings para não trafegar a cada leitura das configurações.
type Backgrounds struct {
	Login string `json:"login"`
	App   string `json:"app"`
}

type Settings struct {
	// StoreName é o nome fantasia (cabeçalho do comprovante e da barra superior).
	StoreName string `json:"storeName"`
	// AutoReceipt abre (ou imprime direto) o comprovante não fiscal após cada venda.
	AutoReceipt bool `json:"autoReceipt"`

	LegalName         string `json:"legalName"`
	Document          string `json:"document"` // CPF ou CNPJ normalizado
	StateRegistration string `json:"stateRegistration"`
	Phone             string `json:"phone"`
	Whatsapp          string `json:"whatsapp"`
	Email             string `json:"email"`
	ZipCode           string `json:"zipCode"`
	Street            string `json:"street"`
	Number            string `json:"number"`
	Complement        string `json:"complement"`
	District          string `json:"district"`
	City              string `json:"city"`
	State             string `json:"state"`
	// Logo é uma imagem PNG/JPEG em data URL. Fica no banco para entrar no backup.
	Logo              string `json:"logo"`
	ReceiptFooter     string `json:"receiptFooter"`
	QuoteValidityDays int64  `json:"quoteValidityDays"`
	// LowStockDefault: estoque baixo para produto sem mínimo próprio (0 = só avisa zerado).
	LowStockDefault int64 `json:"lowStockDefault"`

	// PaperWidth: "58", "80" (térmica, impressão direta) ou "a4" (diálogo do sistema).
	PaperWidth  string `json:"paperWidth"`
	PrinterName string `json:"printerName"`
	CutPaper    bool   `json:"cutPaper"`
	OpenDrawer  bool   `json:"openDrawer"`

	// LockMinutes bloqueia a tela após N minutos sem uso (0 = nunca).
	LockMinutes int64 `json:"lockMinutes"`
	// OperatorMaxDiscountPct: acima deste desconto (%) o operador precisa de autorização.
	OperatorMaxDiscountPct int64 `json:"operatorMaxDiscountPct"`

	// O que entra no custo do produto ao importar o XML da NF-e (valores por item da nota).
	NFeCostFreight  bool `json:"nfeCostFreight"`
	NFeCostIPI      bool `json:"nfeCostIpi"`
	NFeCostST       bool `json:"nfeCostSt"`
	NFeCostOther    bool `json:"nfeCostOther"` // seguro e outras despesas
	NFeCostDiscount bool `json:"nfeCostDiscount"`

	// Pix estático: chave da loja, nome do recebedor e cidade (sem acento, até 25/15).
	PixKey  string `json:"pixKey"`
	PixName string `json:"pixName"`
	PixCity string `json:"pixCity"`

	// ChargeMessage: cobrança do a prazo pelo WhatsApp; aceita {nome}, {saldo} e {loja}.
	ChargeMessage string `json:"chargeMessage"`
	// SetupDone: o assistente de primeira configuração foi concluído ou pulado.
	SetupDone bool `json:"setupDone"`
	// CashPerUser: cada usuário abre e fecha o próprio caixa (falso = caixa geral).
	CashPerUser bool `json:"cashPerUser"`

	// Mensagens do WhatsApp (aceitam {nome}, {plano}, {vencimento}, {valor}, {loja}).
	MsgBirthday string `json:"msgBirthday"`
	MsgDue      string `json:"msgDue"`     // mensalidade a vencer
	MsgOverdue  string `json:"msgOverdue"` // mensalidade vencida
	MsgMissing  string `json:"msgMissing"` // inativo (vencida há muito tempo)
	MsgWinback  string `json:"msgWinback"` // assinatura cancelada
	// DueDays: "a vencer" = vence nos próximos N dias; MissingDays: "inativo" =
	// vencida há mais de N dias.
	DueDays     int64 `json:"dueDays"`
	MissingDays int64 `json:"missingDays"`

	// Backup automático diário em <dados>/backups, guardando os últimos BackupKeep.
	BackupAuto bool  `json:"backupAuto"`
	BackupKeep int64 `json:"backupKeep"`
	// BackupDir: onde o backup automático é salvo (vazio = <dados>/backups). O banco não muda de lugar.
	BackupDir string `json:"backupDir"`
	// BackupExternalDir: pendrive ou pasta sincronizada (vazio = sem cópia externa).
	BackupExternalDir string `json:"backupExternalDir"`
	// BackupWarnDays: o Painel avisa se a última cópia externa tem mais de N dias.
	BackupWarnDays int64 `json:"backupWarnDays"`
	// PixOnReceipt imprime o QR do valor em fiado no cupom (para o cliente pagar depois).
	PixOnReceipt bool `json:"pixOnReceipt"`
}

// PixCode é o "copia e cola" e a imagem do QR de um valor.
type PixCode struct {
	Payload string `json:"payload"`
	Image   string `json:"image"` // PNG em data URL
}

// Papéis de usuário.
const (
	RoleAdmin    = "admin"
	RoleManager  = "gerente"
	RoleOperator = "operador"
)

type User struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Login     string    `json:"login"`
	Role      string    `json:"role"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Permissions são as permissões efetivas (para o administrador, todas).
	Permissions []string `json:"permissions"`
	// LoggedIn: há uma sessão aberta com este usuário (aqui ou em outro lugar).
	LoggedIn  bool   `json:"loggedIn"`
	SessionID string `json:"-"`
}

// UserInput: Password vazio na edição mantém a senha atual.
type UserInput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Login    string `json:"login"`
	Password string `json:"password"`
	Role     string `json:"role"`
	Active   bool   `json:"active"`
	// Permissions escolhidas pelo administrador; nil usa o padrão do papel.
	Permissions []string `json:"permissions"`
}

// PermissionItem é uma permissão que o administrador pode marcar por usuário.
type PermissionItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// PermissionCatalog alimenta a tela: a lista e os padrões de cada papel.
type PermissionCatalog struct {
	Items    []PermissionItem    `json:"items"`
	Defaults map[string][]string `json:"defaults"`
}

// Session é o estado de login mostrado pelo frontend.
type Session struct {
	User        *User    `json:"user"`
	NeedsSetup  bool     `json:"needsSetup"` // nenhum usuário cadastrado ainda
	Permissions []string `json:"permissions"`
}

type AuditEntry struct {
	ID           int64     `json:"id"`
	UserName     *string   `json:"userName"`
	AuthorizedBy *string   `json:"authorizedBy"`
	Action       string    `json:"action"`
	Entity       *string   `json:"entity"`
	EntityID     *int64    `json:"entityId"`
	BeforeJSON   *string   `json:"beforeJson"`
	AfterJSON    *string   `json:"afterJson"`
	CreatedAt    time.Time `json:"createdAt"`
}

// CategoryTotal soma os itens vendidos de uma categoria (pela categoria atual do produto).
type CategoryTotal struct {
	CategoryName string `json:"categoryName"`
	Qty          int64  `json:"qty"`
	TotalCents   int64  `json:"totalCents"`
}

// OperatorTotal soma as vendas concluídas de um operador.
type OperatorTotal struct {
	UserName   string `json:"userName"`
	Count      int64  `json:"count"`
	TotalCents int64  `json:"totalCents"`
}

// Tipos de comprovante não fiscal.
const (
	ReceiptSale          = "venda"
	ReceiptCreditPayment = "recebimento"
	ReceiptCashSession   = "caixa"
	ReceiptCashMovement  = "movimento"
	ReceiptTest          = "teste"
	ReceiptQuote         = "orcamento"
	ReceiptReport        = "relatorio"
	ReceiptReturn        = "devolucao"
)

// Tipos de linha do comprovante.
const (
	LineText      = "text"      // Text, com Bold/Center
	LinePair      = "pair"      // Text à esquerda, Right à direita
	LineItem      = "item"      // item numa linha só: Text (produto), Qty, Price e Right (total)
	LineSeparator = "sep"       // linha tracejada
	LineSignature = "signature" // espaço e traço para assinatura, Text como legenda
	LineQR        = "qr"        // QR Code: Text = conteúdo, Right = imagem PNG (data URL) para a tela
)

// ReceiptLine é uma linha neutra: o mesmo comprovante é desenhado em HTML
// (tela/A4) e em ESC/POS (impressora térmica).
type ReceiptLine struct {
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Right  string `json:"right"`
	Bold   bool   `json:"bold"`
	Center bool   `json:"center"`
	// Só em LineItem: quantidade e preço unitário já formatados.
	Qty   string `json:"qty"`
	Price string `json:"price"`
}

type Receipt struct {
	Title string        `json:"title"`
	Logo  string        `json:"logo"`
	Lines []ReceiptLine `json:"lines"`
}

type AppInfo struct {
	Version       string `json:"version"`
	DataDir       string `json:"dataDir"`
	DatabasePath  string `json:"databasePath"`
	SchemaVersion int    `json:"schemaVersion"`
}

// BackupStatus é o estado do backup automático e da cópia externa (Painel e Configurações).
type BackupStatus struct {
	AutoEnabled   bool       `json:"autoEnabled"`
	AutoDir       string     `json:"autoDir"` // pasta em uso (a padrão, se nenhuma escolhida)
	AutoDirError  string     `json:"autoDirError"`
	LastAuto      *time.Time `json:"lastAuto"`
	ExternalDir   string     `json:"externalDir"`
	LastExternal  *time.Time `json:"lastExternal"`
	ExternalError string     `json:"externalError"`
	// Warning: texto do alerta do Painel (vazio = tudo em dia).
	Warning string `json:"warning"`
}

type BackupResult struct {
	Path      string    `json:"path"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
}

// ---------- Análises (Fase 5) ----------

// ProfitRow: lucro bruto de um produto ou categoria no período.
type ProfitRow struct {
	ProductID    int64  `json:"productId"` // 0 nas linhas de categoria
	Name         string `json:"name"`
	CategoryName string `json:"categoryName"`
	Qty          int64  `json:"qty"`          // vendido menos devolvido
	RevenueCents int64  `json:"revenueCents"` // o que o cliente pagou, menos devoluções
	CostCents    int64  `json:"costCents"`
	ProfitCents  int64  `json:"profitCents"`
	Estimated    bool   `json:"estimated"` // usou o custo atual (venda antiga sem custo gravado)
	NoCost       bool   `json:"noCost"`    // produto sem custo cadastrado
}

type ProfitReport struct {
	Products     []ProfitRow `json:"products"`
	Categories   []ProfitRow `json:"categories"`
	RevenueCents int64       `json:"revenueCents"`
	CostCents    int64       `json:"costCents"`
	ProfitCents  int64       `json:"profitCents"`
}

type ABCRow struct {
	ProductID     int64   `json:"productId"`
	Name          string  `json:"name"`
	Qty           int64   `json:"qty"`
	RevenueCents  int64   `json:"revenueCents"`
	SharePct      float64 `json:"sharePct"`
	CumulativePct float64 `json:"cumulativePct"`
	Class         string  `json:"class"` // A, B ou C
}

type ABCReport struct {
	Rows       []ABCRow `json:"rows"`
	TotalCents int64    `json:"totalCents"`
}

type IdleProduct struct {
	ProductID    int64      `json:"productId"`
	Name         string     `json:"name"`
	CategoryName string     `json:"categoryName"`
	StockQty     int64      `json:"stockQty"`
	CostCents    int64      `json:"costCents"` // unitário; 0 para quem não vê custo
	PriceCents   int64      `json:"priceCents"`
	LastSaleAt   *time.Time `json:"lastSaleAt"` // nil = nunca vendeu
	IdleDays     int64      `json:"idleDays"`   // -1 = nunca vendeu
}

// AgingCustomer: dívida do fiado separada pela idade das compras.
type AgingCustomer struct {
	CustomerID   int64   `json:"customerId"`
	Name         string  `json:"name"`
	Phone        *string `json:"phone"`
	BalanceCents int64   `json:"balanceCents"`
	Upto30Cents  int64   `json:"upto30Cents"`
	Upto60Cents  int64   `json:"upto60Cents"`
	Upto90Cents  int64   `json:"upto90Cents"`
	Over90Cents  int64   `json:"over90Cents"`
	OldestDays   int64   `json:"oldestDays"` // idade da compra mais antiga ainda em aberto
}

type SupplierPurchases struct {
	SupplierID int64      `json:"supplierId"`
	Name       string     `json:"name"`
	Count      int64      `json:"count"`
	TotalCents int64      `json:"totalCents"`
	LastAt     *time.Time `json:"lastAt"`
}

type StockValueRow struct {
	CategoryName string `json:"categoryName"`
	Products     int64  `json:"products"`
	Qty          int64  `json:"qty"`
	CostCents    int64  `json:"costCents"`  // 0 para quem não vê custo
	PriceCents   int64  `json:"priceCents"` // a preço de venda
	NoCost       int64  `json:"noCost"`     // produtos sem custo cadastrado
}

type DiscountByOperator struct {
	UserName          string `json:"userName"`
	Sales             int64  `json:"sales"`
	SalesWithDiscount int64  `json:"salesWithDiscount"`
	SubtotalCents     int64  `json:"subtotalCents"`
	DiscountCents     int64  `json:"discountCents"` // dado pelo operador (item + geral)
	PromoCents        int64  `json:"promoCents"`    // das promoções
}

// ExportTable: qualquer listagem da tela exportada para o Excel.
type ExportColumn struct {
	Label string `json:"label"`
	Kind  string `json:"kind"` // text, int, money (centavos), pct (fração: 0.25 = 25%)
}

type ExportTable struct {
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle"`
	Columns  []ExportColumn `json:"columns"`
	Rows     [][]string     `json:"rows"`
}

// Delivery: endereço de entrega de uma venda (só para imprimir no cupom).
type Delivery struct {
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Notes   string `json:"notes"`
}

// ---------- Licença (Fase 7) ----------

// LicenseStatus: situação da licença desta máquina. Sem CanWrite o sistema fica
// em modo somente consulta (vê tudo e faz backup, não grava movimento).
type LicenseStatus struct {
	// State: ativa, teste, vencida, teste_encerrado ou relogio.
	State     string `json:"state"`
	CanWrite  bool   `json:"canWrite"`
	MachineID string `json:"machineId"`
	Store     string `json:"store"`
	Document  string `json:"document"`
	Plan      string `json:"plan"`
	// Expires: AAAA-MM-DD (licença com validade ou fim do teste); vazio = vitalícia.
	Expires  string `json:"expires"`
	DaysLeft int    `json:"daysLeft"`
	Message  string `json:"message"`
	// Warning: aviso para a barra superior e o Painel (vazio = nada a avisar).
	Warning string `json:"warning"`
	// LicenseError: a licença gravada não vale aqui (ex.: é de outra máquina).
	LicenseError string `json:"licenseError"`
}

// ---------- Importação de planilhas (Fase 7) ----------

// ImportRequest: arquivo escolhido, coluna de cada campo e se atualiza os já cadastrados.
type ImportRequest struct {
	// Kind: produtos ou clientes.
	Kind string `json:"kind"`
	Path string `json:"path"`
	// Mapping: campo → número da coluna (0 = primeira). Campo ausente não é importado.
	Mapping map[string]int `json:"mapping"`
	// Update: atualiza quem já existe (produto pelo código de barras ou SKU; cliente pelo CPF/CNPJ).
	Update bool `json:"update"`
	// SupplierID: fornecedor da entrada de mercadoria criada com o estoque dos
	// produtos novos (0 = fornecedor genérico "Sem fornecedor").
	SupplierID int64 `json:"supplierId"`
}

// ImportField é um campo que pode receber uma coluna da planilha.
type ImportField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// ImportFile: o arquivo lido, para a tela de mapeamento.
type ImportFile struct {
	Path    string         `json:"path"`
	Name    string         `json:"name"`
	Headers []string       `json:"headers"`
	Sample  [][]string     `json:"sample"`
	Rows    int            `json:"rows"`
	Fields  []ImportField  `json:"fields"`
	Mapping map[string]int `json:"mapping"` // sugerido pelos nomes das colunas
}

// ImportRow: resultado da validação de uma linha (Line = linha na planilha).
type ImportRow struct {
	Line int    `json:"line"`
	Name string `json:"name"`
	// Action: novo, atualizar, ignorar ou erro.
	Action  string `json:"action"`
	Message string `json:"message"`
}

type ImportPreview struct {
	Rows    []ImportRow `json:"rows"`
	New     int         `json:"new"`
	Updated int         `json:"updated"`
	Skipped int         `json:"skipped"`
	Errors  int         `json:"errors"`
}
