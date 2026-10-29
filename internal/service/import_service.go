// Caminho: pdv/internal/service/import_service.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"pdv/internal/importer"
	"pdv/internal/models"
	"pdv/internal/repository"
)

// Importação de produtos e clientes por planilha. O fluxo é sempre:
// Open (lê e sugere as colunas) → Preview (valida linha a linha, não grava) →
// Confirm (valida de novo e grava tudo numa transação; com qualquer erro, nada
// é gravado). A primeira linha da planilha é sempre o cabeçalho.
type ImportService struct {
	Base
	products   repository.ProductRepo
	categories repository.CategoryRepo
	customers  repository.CustomerRepo
	suppliers  repository.SupplierRepo
}

func NewImportService(b Base) *ImportService { return &ImportService{Base: b} }

const (
	importProducts  = "produtos"
	importCustomers = "clientes"
)

var importFields = map[string][]models.ImportField{
	importProducts: {
		{Key: "nome", Label: "Nome", Required: true}, {Key: "codigo_barras", Label: "Código de barras"}, {Key: "sku", Label: "SKU"},
		{Key: "categoria", Label: "Categoria"}, {Key: "custo", Label: "Preço de custo"}, {Key: "preco", Label: "Preço de venda", Required: true},
		{Key: "estoque", Label: "Estoque"}, {Key: "estoque_minimo", Label: "Estoque mínimo"},
	},
	importCustomers: {
		{Key: "nome", Label: "Nome", Required: true}, {Key: "telefone", Label: "Telefone"}, {Key: "documento", Label: "CPF/CNPJ"},
		{Key: "email", Label: "E-mail"}, {Key: "cep", Label: "CEP"}, {Key: "rua", Label: "Rua"}, {Key: "numero", Label: "Número"},
		{Key: "complemento", Label: "Complemento"}, {Key: "bairro", Label: "Bairro"}, {Key: "cidade", Label: "Cidade"},
		{Key: "uf", Label: "UF"}, {Key: "limite", Label: "Limite a prazo"},
	},
}

// importSynonyms: nomes de coluna comuns (sem acento, minúsculos). A ordem dos
// campos importa: os mais específicos primeiro ("estoque minimo" antes de "estoque").
var importSynonyms = map[string][][2]string{
	importProducts: {
		{"estoque_minimo", "estoque minimo|minimo|est. minimo|estoque min"},
		{"custo", "custo|preco de custo|preco custo|valor de custo"},
		{"codigo_barras", "codigo de barras|codigo barras|cod barras|ean|gtin|codigo"},
		{"sku", "sku|codigo interno|referencia|ref"},
		{"categoria", "categoria|grupo|departamento|secao"},
		{"preco", "preco de venda|preco venda|preco|valor|valor de venda"},
		{"estoque", "estoque|quantidade|qtd|qtde|saldo"},
		{"nome", "nome|produto|descricao|nome do produto"},
	},
	importCustomers: {
		{"documento", "cpf/cnpj|cpf|cnpj|documento|cpf ou cnpj"},
		{"telefone", "telefone|celular|whatsapp|fone"},
		{"email", "e-mail|email"},
		{"cep", "cep"},
		{"numero", "numero|nº|n"},
		{"complemento", "complemento"},
		{"bairro", "bairro"},
		{"cidade", "cidade|municipio"},
		{"uf", "uf|estado"},
		{"rua", "rua|endereco|logradouro"},
		{"limite", "limite a prazo|limite|limite de credito|limite fiado"},
		{"nome", "nome|cliente|nome do cliente|razao social"},
	},
}

var foldChain = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

func foldText(s string) string {
	out, _, err := transform.String(foldChain, s)
	if err != nil {
		out = s
	}
	return strings.ToLower(strings.Join(strings.Fields(out), " "))
}

// headerNote: "(opcional)", "(obrigatório)" e afins no fim do cabeçalho não contam.
var headerNote = regexp.MustCompile(`\s*\([^)]*\)\s*$`)

// guessMapping liga cada campo a uma coluna pelo nome do cabeçalho: primeiro
// nome exato, depois "contém".
func guessMapping(kind string, headers []string) map[string]int {
	out := map[string]int{}
	used := map[int]bool{}
	folded := make([]string, len(headers))
	for i, h := range headers {
		folded[i] = foldText(headerNote.ReplaceAllString(h, ""))
	}
	for _, exact := range []bool{true, false} {
		for _, fs := range importSynonyms[kind] {
			if _, done := out[fs[0]]; done {
				continue
			}
		cols:
			for i, h := range folded {
				if used[i] || h == "" {
					continue
				}
				for _, syn := range strings.Split(fs[1], "|") {
					if h == syn || (!exact && len(syn) > 2 && strings.Contains(h, syn)) {
						out[fs[0]], used[i] = i, true
						break cols
					}
				}
			}
		}
	}
	return out
}

func readImportFile(path string) ([][]string, error) {
	if !filepath.IsAbs(path) {
		return nil, invalid("Escolha o arquivo pelo botão \"Escolher arquivo\".")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, invalid("Não foi possível abrir o arquivo %s.", filepath.Base(path))
	}
	if fi.Size() > importer.MaxFileBytes {
		return nil, invalid("O arquivo é grande demais (máximo %d MB).", importer.MaxFileBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, invalid("Não foi possível ler o arquivo %s.", filepath.Base(path))
	}
	rows, err := importer.Read(path, data)
	if err != nil {
		return nil, invalid("Não foi possível ler a planilha: %s.", err.Error())
	}
	if len(rows) < 2 {
		return nil, invalid("A planilha está vazia: precisa do cabeçalho e de pelo menos uma linha.")
	}
	return rows, nil
}

// Open lê o arquivo e sugere a coluna de cada campo.
func (s *ImportService) Open(ctx context.Context, kind, path string) (models.ImportFile, error) {
	fields, ok := importFields[kind]
	if !ok {
		return models.ImportFile{}, invalid("Tipo de importação inválido.")
	}
	rows, err := readImportFile(path)
	if err != nil {
		return models.ImportFile{}, err
	}
	sample := rows[1:min(len(rows), 6)]
	return models.ImportFile{Path: path, Name: filepath.Base(path), Headers: rows[0], Sample: sample, Rows: len(rows) - 1,
		Fields: fields, Mapping: guessMapping(kind, rows[0])}, nil
}

// ---------- conversão das células ----------

func parseMoneyCell(v string) (int64, error) {
	t := strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(v), "R$"), " ", "")
	if strings.Contains(t, ",") { // 1.234,56
		t = strings.Replace(strings.ReplaceAll(t, ".", ""), ",", ".", 1)
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || f*100 > float64(MaxPriceCents) {
		return 0, fmt.Errorf("valor inválido: %q", v)
	}
	return int64(math.Round(f * 100)), nil
}

var thousands = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)

func parseIntCell(v string) (int64, error) {
	t := strings.TrimSpace(v)
	if thousands.MatchString(t) { // 1.000
		t = strings.ReplaceAll(t, ".", "")
	}
	f, err := strconv.ParseFloat(strings.Replace(t, ",", ".", 1), 64)
	if err != nil || f != math.Trunc(f) || f < 0 || f > float64(MaxQty) {
		return 0, fmt.Errorf("quantidade inválida: %q (use número inteiro de 0 a %d)", v, MaxQty)
	}
	return int64(f), nil
}

func msgOf(err error) string {
	_, m, _ := Public(err)
	return m
}

// importPlan: o que a planilha vai fazer, linha a linha.
type importPlan struct {
	prev      models.ImportPreview
	products  []productOp
	customers []customerOp
}

type productOp struct {
	ex       *models.Product
	in       models.ProductInput
	category string // vazio = mantém (atualização) ou sem categoria (novo)
	stock    *int64 // atualização: saldo desejado
}

type customerOp struct {
	ex *models.Customer
	in models.CustomerInput
}

func (p *importPlan) add(row models.ImportRow) {
	switch row.Action {
	case "novo":
		p.prev.New++
	case "atualizar":
		p.prev.Updated++
	case "ignorar":
		p.prev.Skipped++
	case "erro":
		p.prev.Errors++
	}
	p.prev.Rows = append(p.prev.Rows, row)
}

func (s *ImportService) plan(ctx context.Context, q repository.DBTX, rows [][]string, req models.ImportRequest) (importPlan, error) {
	switch req.Kind {
	case importProducts:
		return s.planProducts(ctx, q, rows, req)
	case importCustomers:
		return s.planCustomers(ctx, q, rows, req)
	}
	return importPlan{}, invalid("Tipo de importação inválido.")
}

func cellOf(r []string, mapping map[string]int) func(string) string {
	return func(k string) string {
		c, ok := mapping[k]
		if !ok || c < 0 || c >= len(r) {
			return ""
		}
		return strings.TrimSpace(r[c])
	}
}

func (s *ImportService) planProducts(ctx context.Context, q repository.DBTX, rows [][]string, req models.ImportRequest) (importPlan, error) {
	var p importPlan
	if _, ok := req.Mapping["nome"]; !ok {
		return p, invalid("Escolha a coluna do nome do produto.")
	}
	all, err := s.products.List(ctx, q)
	if err != nil {
		return p, err
	}
	byBarcode, bySku := map[string]*models.Product{}, map[string]*models.Product{}
	for i := range all {
		if b := all[i].Barcode; b != nil {
			byBarcode[*b] = &all[i]
		}
		if k := all[i].Sku; k != nil {
			bySku[*k] = &all[i]
		}
	}
	cats, err := s.categories.List(ctx, q)
	if err != nil {
		return p, err
	}
	known := map[string]bool{}
	for _, c := range cats {
		known[foldText(c.Name)] = true
	}
	seenBC, seenSku := map[string]int{}, map[string]int{}

	for i, r := range rows[1:] {
		cell := cellOf(r, req.Mapping)
		row := models.ImportRow{Line: i + 2, Name: cell("nome")}
		fail := func(format string, a ...any) {
			row.Action, row.Message = "erro", fmt.Sprintf(format, a...)
			p.add(row)
		}
		// Linha do modelo que ficou só com o código reservado: não é produto.
		onlyCodes := true
		for k := range req.Mapping {
			if k != "codigo_barras" && k != "sku" && cell(k) != "" {
				onlyCodes = false
				break
			}
		}
		if onlyCodes {
			continue
		}
		bc, sku := cell("codigo_barras"), cell("sku")
		if l, ok := seenBC[bc]; ok && bc != "" {
			fail("Código de barras %s repetido (linha %d).", bc, l)
			continue
		}
		if l, ok := seenSku[sku]; ok && sku != "" {
			fail("SKU %s repetido (linha %d).", sku, l)
			continue
		}
		if bc != "" {
			seenBC[bc] = row.Line
		}
		if sku != "" {
			seenSku[sku] = row.Line
		}
		ex := byBarcode[bc]
		if other := bySku[sku]; sku != "" && other != nil {
			if ex != nil && ex.ID != other.ID {
				fail("O código de barras é de \"%s\" e o SKU é de \"%s\".", ex.Name, other.Name)
				continue
			}
			ex = other
		}
		if ex != nil && !req.Update {
			row.Name, row.Action, row.Message = ex.Name, "ignorar", "Já cadastrado (marque \"atualizar os já cadastrados\" para alterar)."
			p.add(row)
			continue
		}

		in := models.ProductInput{Active: true}
		if ex != nil {
			cost := ex.CostCents
			in = models.ProductInput{ID: ex.ID, Name: ex.Name, Barcode: ex.Barcode, Sku: ex.Sku, PriceCents: ex.PriceCents,
				Active: ex.Active, CategoryID: ex.CategoryID, CostCents: &cost, MinStock: ex.MinStock}
		}
		if v := cell("nome"); v != "" {
			in.Name = v
		} else if ex == nil {
			fail("Nome em branco.")
			continue
		}
		if bc != "" {
			in.Barcode = &bc
		}
		if sku != "" {
			in.Sku = &sku
		}
		if v := cell("preco"); v != "" {
			if in.PriceCents, err = parseMoneyCell(v); err != nil {
				fail("Preço de venda %s.", err)
				continue
			}
		} else if ex == nil {
			fail("Preço de venda em branco.")
			continue
		}
		if v := cell("custo"); v != "" {
			c, err := parseMoneyCell(v)
			if err != nil {
				fail("Custo %s.", err)
				continue
			}
			in.CostCents = &c
		}
		if v := cell("estoque_minimo"); v != "" {
			if in.MinStock, err = parseIntCell(v); err != nil {
				fail("Estoque mínimo: %s.", err)
				continue
			}
		}
		op := productOp{ex: ex, category: cell("categoria")}
		if v := cell("estoque"); v != "" {
			n, err := parseIntCell(v)
			if err != nil {
				fail("Estoque: %s.", err)
				continue
			}
			if ex == nil {
				in.InitialStock = n
			} else if n != ex.StockQty {
				op.stock = &n
			}
		}
		if _, err := validateProduct(in); err != nil {
			fail("%s", msgOf(err))
			continue
		}
		op.in = in
		row.Name, row.Action = in.Name, "novo"
		if ex != nil {
			row.Action = "atualizar"
			if op.stock != nil {
				row.Message = fmt.Sprintf("Estoque de %d para %d (ajuste).", ex.StockQty, *op.stock)
			}
		} else if in.InitialStock > 0 {
			row.Message = fmt.Sprintf("Entrada de mercadoria: %d un.", in.InitialStock)
		}
		if op.category != "" && !known[foldText(op.category)] {
			row.Message = strings.TrimSpace(row.Message + " Categoria nova: " + op.category + ".")
		}
		p.products = append(p.products, op)
		p.add(row)
	}
	return p, nil
}

func (s *ImportService) planCustomers(ctx context.Context, q repository.DBTX, rows [][]string, req models.ImportRequest) (importPlan, error) {
	var p importPlan
	if _, ok := req.Mapping["nome"]; !ok {
		return p, invalid("Escolha a coluna do nome do cliente.")
	}
	all, err := s.customers.List(ctx, q, "")
	if err != nil {
		return p, err
	}
	byDoc := map[string]*models.Customer{}
	for i := range all {
		if d := all[i].Document; d != nil {
			byDoc[*d] = &all[i]
		}
	}
	seen := map[string]int{}
	for i, r := range rows[1:] {
		cell := cellOf(r, req.Mapping)
		row := models.ImportRow{Line: i + 2, Name: cell("nome")}
		fail := func(format string, a ...any) {
			row.Action, row.Message = "erro", fmt.Sprintf(format, a...)
			p.add(row)
		}
		doc := NormalizeDocument(cell("documento"))
		if l, ok := seen[doc]; ok && doc != "" {
			fail("CPF/CNPJ repetido (linha %d).", l)
			continue
		}
		if doc != "" {
			seen[doc] = row.Line
		}
		ex := byDoc[doc]
		if ex != nil && !req.Update {
			row.Name, row.Action, row.Message = ex.Name, "ignorar", "Já cadastrado (marque \"atualizar os já cadastrados\" para alterar)."
			p.add(row)
			continue
		}
		in := models.CustomerInput{Active: true}
		if ex != nil {
			in = models.CustomerInput{ID: ex.ID, Name: ex.Name, Phone: ex.Phone, Document: ex.Document, Email: ex.Email,
				ZipCode: ex.ZipCode, Street: ex.Street, Number: ex.Number, Complement: ex.Complement, District: ex.District,
				City: ex.City, State: ex.State, Reference: ex.Reference, Notes: ex.Notes, CreditLimitCents: ex.CreditLimitCents, Active: ex.Active}
		}
		if v := cell("nome"); v != "" {
			in.Name = v
		}
		for key, dst := range map[string]**string{"telefone": &in.Phone, "documento": &in.Document, "email": &in.Email,
			"cep": &in.ZipCode, "rua": &in.Street, "numero": &in.Number, "complemento": &in.Complement, "bairro": &in.District,
			"cidade": &in.City, "uf": &in.State} {
			if v := cell(key); v != "" {
				*dst = &v
			}
		}
		if v := cell("limite"); v != "" {
			if in.CreditLimitCents, err = parseMoneyCell(v); err != nil {
				fail("Limite a prazo %s.", err)
				continue
			}
		}
		if _, err := validateCustomer(in); err != nil {
			fail("%s", msgOf(err))
			continue
		}
		row.Name, row.Action = in.Name, "novo"
		if ex != nil {
			row.Action = "atualizar"
		}
		p.customers = append(p.customers, customerOp{ex: ex, in: in})
		p.add(row)
	}
	return p, nil
}

// Preview valida a planilha inteira sem gravar nada.
func (s *ImportService) Preview(ctx context.Context, req models.ImportRequest) (models.ImportPreview, error) {
	rows, err := readImportFile(req.Path)
	if err != nil {
		return models.ImportPreview{}, err
	}
	p, err := s.plan(ctx, s.DB, rows, req)
	return p.prev, err
}

// Confirm grava tudo numa transação. Com qualquer linha com erro, nada é gravado.
func (s *ImportService) Confirm(ctx context.Context, req models.ImportRequest) (models.ImportPreview, error) {
	rows, err := readImportFile(req.Path)
	if err != nil {
		return models.ImportPreview{}, err
	}
	var out models.ImportPreview
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		p, err := s.plan(ctx, tx, rows, req)
		if err != nil {
			return err
		}
		if p.prev.Errors > 0 {
			return invalid("Há %d linha(s) com erro. Corrija a planilha (ou deixe essas linhas de fora) e confira de novo.", p.prev.Errors)
		}
		if p.prev.New+p.prev.Updated == 0 {
			return invalid("Nada para importar: todas as linhas já estão cadastradas.")
		}
		if req.Kind == importProducts {
			err = s.applyProducts(ctx, tx, p, req)
		} else {
			err = s.applyCustomers(ctx, tx, p)
		}
		out = p.prev
		return err
	})
	return out, err
}

func (s *ImportService) applyProducts(ctx context.Context, tx *sql.Tx, p importPlan, req models.ImportRequest) error {
	now := s.Now()
	authBy, err := requirePerm(ctx, now, PermProductCreate)
	if err != nil {
		return err
	}
	if _, mapped := req.Mapping["custo"]; mapped {
		if _, err := requirePerm(ctx, now, PermCost); err != nil {
			return err
		}
	}
	needPrice, needAdjust := false, false
	for _, op := range p.products {
		needPrice = needPrice || (op.ex != nil && op.ex.PriceCents != op.in.PriceCents)
		needAdjust = needAdjust || op.stock != nil
	}
	if needPrice {
		if _, err := requirePerm(ctx, now, PermPrice); err != nil {
			return err
		}
	}
	if needAdjust {
		if _, err := requirePerm(ctx, now, PermStockAdjust); err != nil {
			return err
		}
	}
	cats, err := s.categories.List(ctx, tx)
	if err != nil {
		return err
	}
	catID := map[string]int64{}
	for _, c := range cats {
		catID[foldText(c.Name)] = c.ID
	}
	note := ptr("Importação de planilha: " + filepath.Base(req.Path))
	// O estoque dos produtos novos vira uma entrada de mercadoria (aparece na
	// lista de entradas, pode ser estornada e serve para as etiquetas).
	var entry []models.PurchaseItemInput
	for _, op := range p.products {
		f, err := validateProduct(op.in)
		if err != nil {
			return err
		}
		if op.category != "" {
			id, ok := catID[foldText(op.category)]
			if !ok {
				name, err := cleanName(op.category, "nome da categoria")
				if err != nil {
					return err
				}
				if id, err = s.categories.Insert(ctx, tx, name, now); err != nil {
					return err
				}
				catID[foldText(op.category)] = id
			}
			f.CategoryID = &id
		}
		if op.ex == nil {
			id, err := s.products.Insert(ctx, tx, f, now)
			if err != nil {
				return fromRepo(err, nil, duplicateBarcodeMsg(f))
			}
			if op.in.InitialStock > 0 {
				entry = append(entry, models.PurchaseItemInput{ProductID: id, Qty: op.in.InitialStock, UnitCostCents: f.CostCents})
			}
			continue
		}
		f.PricePending = op.ex.PricePending && op.ex.PriceCents == f.PriceCents
		if err := s.products.Update(ctx, tx, op.ex.ID, f, now); err != nil {
			return fromRepo(err, nil, duplicateBarcodeMsg(f))
		}
		if op.stock != nil && !op.ex.NoStock {
			if err := applyStockChange(ctx, tx, now, repository.NewMovement{ProductID: op.ex.ID, QtyChange: *op.stock - op.ex.StockQty,
				Reason: models.StockAdjust, Notes: note}); err != nil {
				return err
			}
		}
	}
	if err := s.importEntry(ctx, tx, entry, req); err != nil {
		return err
	}
	return audit(ctx, tx, now, authBy, "importacao_produtos", "produto", nil, nil,
		map[string]any{"arquivo": filepath.Base(req.Path), "novos": p.prev.New, "atualizados": p.prev.Updated})
}

// noSupplierName: fornecedor genérico das entradas importadas sem fornecedor escolhido.
const noSupplierName = "Sem fornecedor"

// importEntry registra o estoque dos produtos novos como entrada(s) de
// mercadoria, em blocos do tamanho máximo de uma entrada.
func (s *ImportService) importEntry(ctx context.Context, tx *sql.Tx, items []models.PurchaseItemInput, req models.ImportRequest) error {
	if len(items) == 0 {
		return nil
	}
	supplierID := req.SupplierID
	if supplierID == 0 {
		list, err := s.suppliers.List(ctx, tx, noSupplierName)
		if err != nil {
			return err
		}
		for _, v := range list {
			if v.LegalName == noSupplierName {
				supplierID = v.ID
				break
			}
		}
		if supplierID == 0 {
			if supplierID, err = s.suppliers.Insert(ctx, tx, models.Supplier{LegalName: noSupplierName, Active: true}, s.Now()); err != nil {
				return err
			}
		}
	}
	purchases := &PurchaseService{Base: s.Base}
	for len(items) > 0 {
		n := min(len(items), maxPurchaseItems)
		in := models.PurchaseInput{SupplierID: supplierID, IssueDate: s.Now().Format("2006-01-02"),
			Notes: "Importação de planilha: " + filepath.Base(req.Path), Items: items[:n]}
		invoice, notes, err := validatePurchase(in)
		if err != nil {
			return err
		}
		if _, err := purchases.createTx(ctx, tx, in, invoice, notes); err != nil {
			return err
		}
		items = items[n:]
	}
	return nil
}

func (s *ImportService) applyCustomers(ctx context.Context, tx *sql.Tx, p importPlan) error {
	now := s.Now()
	var authBy *int64
	for _, op := range p.customers {
		if (op.ex == nil && op.in.CreditLimitCents > 0) || (op.ex != nil && op.ex.CreditLimitCents != op.in.CreditLimitCents) {
			var err error
			if authBy, err = requirePerm(ctx, now, PermCreditLimit); err != nil {
				return err
			}
			break
		}
	}
	for _, op := range p.customers {
		f, err := validateCustomer(op.in)
		if err != nil {
			return err
		}
		if op.ex == nil {
			if _, err := s.customers.Insert(ctx, tx, f, now); err != nil {
				return fromRepo(err, nil, duplicateDocumentMsg)
			}
		} else if err := s.customers.Update(ctx, tx, op.ex.ID, f, now); err != nil {
			return fromRepo(err, nil, duplicateDocumentMsg)
		}
	}
	return audit(ctx, tx, now, authBy, "importacao_clientes", "cliente", nil, nil,
		map[string]any{"novos": p.prev.New, "atualizados": p.prev.Updated})
}

// ImportTemplate gera o modelo .xlsx: cabeçalho com os nomes dos campos e, para
// clientes, uma linha de exemplo. Para produtos, uma linha por código reservado
// (só o SKU; o código de barras fica para o da embalagem); sem exemplo, que
// viraria produto de verdade se ninguém o apagasse.
func ImportTemplate(kind string, codes []models.ProductCode) ([]byte, error) {
	fields, ok := importFields[kind]
	if !ok {
		return nil, invalid("Tipo de importação inválido.")
	}
	examples := map[string][]string{
		importCustomers: {"Maria da Silva", "(11) 98765-4321", "", "maria@exemplo.com", "01001-000", "Praça da Sé", "100", "Apto 12", "Sé", "São Paulo", "SP", "200,00"},
	}
	head := make([]xlsxCell, len(fields))
	widths := make([]float64, len(fields))
	for i, f := range fields {
		note := " (opcional)"
		if f.Required {
			note = " (obrigatório)"
		}
		head[i] = xlsxCell{text: f.Label + note, style: xsHeader}
		widths[i] = 18
	}
	widths[0] = 32
	rows := [][]xlsxCell{head}
	if kind == importCustomers {
		ex := make([]xlsxCell, len(fields))
		for i := range fields {
			ex[i] = xlsxCell{text: examples[kind][i]}
		}
		rows = append(rows, ex)
	}
	for _, c := range codes {
		if kind != importProducts {
			break
		}
		r := make([]xlsxCell, len(fields))
		for i, f := range fields {
			if f.Key == "sku" {
				r[i] = xlsxCell{text: c.Sku}
			}
		}
		rows = append(rows, r)
	}
	return writeXLSX(strings.ToUpper(kind[:1])+kind[1:], widths, nil, rows, true)
}
