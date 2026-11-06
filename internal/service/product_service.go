// Caminho: pdv/internal/service/product_service.go

package service

import (
	"context"
	"database/sql"
	"slices"
	"strings"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
	"pdv/internal/repository"
)

const maxBarcodeLen = 64

type ProductService struct {
	Base
	products   repository.ProductRepo
	categories repository.CategoryRepo
	models     repository.ModelRepo
	settings   repository.SettingsRepo
}

func NewProductService(b Base) *ProductService { return &ProductService{Base: b} }

func cleanBarcode(b *string) (*string, error) {
	b = trimPtr(b)
	if b != nil && len(*b) > maxBarcodeLen {
		return nil, invalid("O código de barras deve ter no máximo %d caracteres.", maxBarcodeLen)
	}
	return b, nil
}

// duplicateBarcodeMsg: o banco não diz qual índice único falhou (código ou SKU).
func duplicateBarcodeMsg(f repository.ProductFields) string {
	switch {
	case f.Barcode != nil && f.Sku != nil:
		return "Já existe um produto com o código de barras " + *f.Barcode + " ou com o SKU " + *f.Sku + "."
	case f.Barcode != nil:
		return "Já existe um produto com o código de barras " + *f.Barcode + "."
	case f.Sku != nil:
		return "Já existe um produto com o SKU " + *f.Sku + "."
	}
	return "Código de barras já cadastrado."
}

// validateProduct limpa e confere os dados cadastrais comuns a cadastro e edição.
func validateProduct(in models.ProductInput) (repository.ProductFields, error) {
	f := repository.ProductFields{PriceCents: in.PriceCents, Active: in.Active, CategoryID: validCustomerOrNil(in.CategoryID),
		MinStock: in.MinStock}
	if in.CostCents != nil {
		f.CostCents = *in.CostCents
		if f.CostCents < 0 || f.CostCents > MaxPriceCents {
			return f, invalid("Custo inválido.")
		}
	}
	if in.MinStock < 0 || in.MinStock > MaxQty {
		return f, invalid("O estoque mínimo deve estar entre 0 e %d.", MaxQty)
	}
	var err error
	if f.Name, err = cleanName(in.Name, "nome do produto"); err != nil {
		return f, err
	}
	if f.Barcode, err = cleanBarcode(in.Barcode); err != nil {
		return f, err
	}
	if f.Sku = trimPtr(in.Sku); f.Sku != nil && len(*f.Sku) > maxBarcodeLen {
		return f, invalid("O SKU deve ter no máximo %d caracteres.", maxBarcodeLen)
	}
	return f, validatePrice(in.PriceCents)
}

// checkCategory: a categoria precisa existir e, se for nova para o produto, estar ativa.
func (s *ProductService) checkCategory(ctx context.Context, tx *sql.Tx, id, current *int64) error {
	if id == nil {
		return nil
	}
	c, err := s.categories.GetByID(ctx, tx, *id)
	if err != nil {
		return fromRepo(err, newErr(CodeNotFound, "Categoria %d não encontrada.", *id), "")
	}
	if !c.Active && (current == nil || *current != *id) {
		return invalid("A categoria %q está desativada.", c.Name)
	}
	return nil
}

// cleanPlan valida a duração do plano de assinatura e a descrição; plano não
// controla estoque. Descrição: nil mantém a atual, "" apaga.
func cleanPlan(in *models.ProductInput) error {
	if in.Description != nil {
		d := strings.TrimSpace(*in.Description)
		if len([]rune(d)) > 500 {
			return invalid("A descrição deve ter no máximo 500 caracteres.")
		}
		in.Description = &d
	}
	if in.PlanDays == nil {
		return nil
	}
	if *in.PlanDays < 1 || *in.PlanDays > 3650 {
		return invalid("A duração do plano deve ser de 1 a 3650 dias.")
	}
	in.NoStock = true
	return nil
}

func samePlan(a, b *int64) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

// Create cadastra o produto. O estoque inicial, se houver, é registrado como
// movimentação de entrada na mesma transação.
// cleanDetails valida unidade, fiscal e localização (nil mantém os atuais).
// NCM, CEST e CFOP aceitam pontos ("0901.21.00"), mas guardam só os dígitos.
func cleanDetails(in *models.ProductInput) error {
	d := in.Details
	if d == nil {
		return nil
	}
	digits := func(label, v string, n int) (string, error) {
		v = strings.NewReplacer(".", "", "-", "", " ", "").Replace(v)
		if v == "" {
			return "", nil
		}
		if len(v) != n || strings.Trim(v, "0123456789") != "" {
			return "", invalid("O %s deve ter %d números.", label, n)
		}
		return v, nil
	}
	var err error
	if d.Ncm, err = digits("NCM", d.Ncm, 8); err != nil {
		return err
	}
	if d.Cest, err = digits("CEST", d.Cest, 7); err != nil {
		return err
	}
	if d.Cfop, err = digits("CFOP", d.Cfop, 4); err != nil {
		return err
	}
	d.Unit = strings.ToUpper(strings.TrimSpace(d.Unit))
	if d.Unit == "" {
		d.Unit = "UN"
	}
	if len([]rune(d.Unit)) > 6 {
		return invalid("A unidade deve ter no máximo 6 letras (ex.: UN, KG, CX).")
	}
	d.Origin = strings.TrimSpace(d.Origin)
	if d.Origin != "" && (len(d.Origin) != 1 || d.Origin < "0" || d.Origin > "8") {
		return invalid("A origem da mercadoria vai de 0 a 8.")
	}
	d.Location = strings.TrimSpace(d.Location)
	if len([]rune(d.Location)) > 100 {
		return invalid("A localização deve ter no máximo 100 caracteres.")
	}
	if d.SupplierID != nil && *d.SupplierID <= 0 {
		d.SupplierID = nil
	}
	return nil
}

// saveDetails grava os dados extras do produto (o fornecedor precisa existir).
func (s *ProductService) saveDetails(ctx context.Context, tx *sql.Tx, id int64, d *models.ProductDetails, now time.Time) error {
	if d == nil {
		return nil
	}
	if d.SupplierID != nil {
		var ok bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM suppliers WHERE id = ?)`, *d.SupplierID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return invalid("Fornecedor não encontrado.")
		}
	}
	return s.products.SetDetails(ctx, tx, id, *d, now)
}

func (s *ProductService) Create(ctx context.Context, in models.ProductInput) (models.Product, error) {
	if err := cleanPlan(&in); err != nil {
		return models.Product{}, err
	}
	if err := cleanDetails(&in); err != nil {
		return models.Product{}, err
	}
	f, err := validateProduct(in)
	if err != nil {
		return models.Product{}, err
	}
	if in.InitialStock < 0 || in.InitialStock > MaxQty {
		return models.Product{}, invalid("O estoque inicial deve estar entre 0 e %d.", MaxQty)
	}
	if in.NoStock && in.InitialStock > 0 {
		return models.Product{}, invalid("Produto que não controla estoque não tem estoque inicial.")
	}
	var out models.Product
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		authBy, err := requirePerm(ctx, now, PermProductCreate)
		if err != nil {
			return err
		}
		if err := s.checkCategory(ctx, tx, f.CategoryID, nil); err != nil {
			return err
		}
		if f.CostCents > 0 {
			if _, err := requirePerm(ctx, now, PermCost); err != nil {
				return err
			}
		}
		if authBy != nil {
			if err := audit(ctx, tx, now, authBy, "produto_cadastrado", "produto", nil, nil,
				map[string]any{"nome": f.Name, "preco": f.PriceCents}); err != nil {
				return err
			}
		}
		if in.AutoSku || in.AutoBarcode {
			seqs, err := s.nextCodes(ctx, tx, 1, true)
			if err != nil {
				return err
			}
			c := seqCode(seqs[0])
			if in.AutoSku {
				f.Sku = &c.Sku
			}
			if in.AutoBarcode {
				f.Barcode = &c.Barcode
			}
		}
		id, err := s.products.Insert(ctx, tx, f, now)
		if err != nil {
			return fromRepo(err, nil, duplicateBarcodeMsg(f))
		}
		if in.NoStock {
			if err := s.products.SetNoStock(ctx, tx, id, true, now); err != nil {
				return err
			}
		}
		if in.PlanDays != nil {
			if err := s.products.SetPlanDays(ctx, tx, id, in.PlanDays, now); err != nil {
				return err
			}
		}
		if in.Description != nil && *in.Description != "" {
			if err := s.products.SetDescription(ctx, tx, id, in.Description, now); err != nil {
				return err
			}
		}
		if err := s.saveDetails(ctx, tx, id, in.Details, now); err != nil {
			return err
		}
		if in.InitialStock > 0 {
			if err := applyStockChange(ctx, tx, now, repository.NewMovement{
				ProductID: id, QtyChange: in.InitialStock, Reason: models.StockEntry,
				ReferenceType: ptr("saldo_inicial"), Notes: ptr("Saldo inicial no cadastro"),
			}); err != nil {
				return err
			}
		}
		out, err = s.products.GetByID(ctx, tx, id)
		return err
	})
	return hideCost(ctx, out), err
}

// Update altera nome, código, preço, categoria e situação. O estoque não muda
// aqui; vendas antigas mantêm nome e preço do momento da venda.
func (s *ProductService) Update(ctx context.Context, in models.ProductInput) (models.Product, error) {
	if err := cleanPlan(&in); err != nil {
		return models.Product{}, err
	}
	if err := cleanDetails(&in); err != nil {
		return models.Product{}, err
	}
	f, err := validateProduct(in)
	if err != nil {
		return models.Product{}, err
	}
	var out models.Product
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		before, err := s.products.GetByID(ctx, tx, in.ID)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", in.ID), "")
		}
		if err := s.checkCategory(ctx, tx, f.CategoryID, before.CategoryID); err != nil {
			return err
		}
		if before.Active != f.Active {
			authBy, err := requirePerm(ctx, s.Now(), PermProductActive)
			if err != nil {
				return err
			}
			if err := audit(ctx, tx, s.Now(), authBy, "produto_ativo_alterado", "produto", &in.ID,
				map[string]any{"ativo": before.Active}, map[string]any{"ativo": f.Active}); err != nil {
				return err
			}
		}
		if in.CostCents == nil {
			f.CostCents = before.CostCents
		} else if f.CostCents != before.CostCents {
			authBy, err := requirePerm(ctx, s.Now(), PermCost)
			if err != nil {
				return err
			}
			if err := audit(ctx, tx, s.Now(), authBy, "custo_alterado", "produto", &in.ID,
				map[string]any{"custo": before.CostCents}, map[string]any{"custo": f.CostCents}); err != nil {
				return err
			}
		}
		// Definir (mudar) o preço tira a marca "a precificar".
		f.PricePending = before.PricePending && before.PriceCents == f.PriceCents
		if before.PriceCents != f.PriceCents {
			authBy, err := requirePerm(ctx, s.Now(), PermPrice)
			if err != nil {
				return err
			}
			if err := audit(ctx, tx, s.Now(), authBy, "preco_alterado", "produto", &in.ID,
				map[string]any{"preco": before.PriceCents}, map[string]any{"preco": f.PriceCents}); err != nil {
				return err
			}
		}
		if err := s.products.Update(ctx, tx, in.ID, f, s.Now()); err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", in.ID), duplicateBarcodeMsg(f))
		}
		if in.NoStock != before.NoStock {
			if err := s.products.SetNoStock(ctx, tx, in.ID, in.NoStock, s.Now()); err != nil {
				return err
			}
		}
		if !samePlan(in.PlanDays, before.PlanDays) {
			if err := s.products.SetPlanDays(ctx, tx, in.ID, in.PlanDays, s.Now()); err != nil {
				return err
			}
		}
		if in.Description != nil && *in.Description != str(before.Description) {
			if err := s.products.SetDescription(ctx, tx, in.ID, trimPtr(in.Description), s.Now()); err != nil {
				return err
			}
		}
		if err := s.saveDetails(ctx, tx, in.ID, in.Details, s.Now()); err != nil {
			return err
		}
		out, err = s.products.GetByID(ctx, tx, in.ID)
		return err
	})
	return hideCost(ctx, out), err
}

// Delete apaga o produto que nunca foi usado (vendas, entradas, orçamentos,
// comandas, devoluções, promoções ou qualquer movimento de estoque). Com
// histórico, apagar quebraria relatórios: desativa e devolve deleted=false.
func (s *ProductService) Delete(ctx context.Context, id int64) (deleted bool, err error) {
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		p, err := s.products.GetByID(ctx, tx, id)
		if err != nil {
			return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", id), "")
		}
		used, err := s.products.HasHistory(ctx, tx, id)
		if err != nil {
			return err
		}
		if !used {
			authBy, err := requirePerm(ctx, now, PermProductCreate)
			if err != nil {
				return err
			}
			if err := s.products.Delete(ctx, tx, id); err != nil {
				return err
			}
			deleted = true
			return audit(ctx, tx, now, authBy, "produto_excluido", "produto", &id,
				map[string]any{"nome": p.Name, "codigo": str(p.Barcode), "sku": str(p.Sku)}, nil)
		}
		if !p.Active {
			return newErr(CodeStateConflict, "\"%s\" tem histórico de movimentação e não pode ser excluído. Ele já está desativado.", p.Name)
		}
		authBy, err := requirePerm(ctx, now, PermProductActive)
		if err != nil {
			return err
		}
		f := repository.ProductFields{Name: p.Name, Barcode: p.Barcode, Sku: p.Sku, PriceCents: p.PriceCents, Active: false,
			CategoryID: p.CategoryID, CostCents: p.CostCents, MinStock: p.MinStock, PricePending: p.PricePending}
		if err := s.products.Update(ctx, tx, id, f, now); err != nil {
			return err
		}
		return audit(ctx, tx, now, authBy, "produto_ativo_alterado", "produto", &id,
			map[string]any{"ativo": true}, map[string]any{"ativo": false})
	})
	return deleted, err
}

// ---------- categorias ----------

func (s *ProductService) ListCategories(ctx context.Context) ([]models.Category, error) {
	return s.categories.List(ctx, s.DB)
}

const duplicateCategoryMsg = "Já existe uma categoria com este nome."

// SaveCategory cria (ID 0) ou altera uma categoria. Exige poder cadastrar produto.
func (s *ProductService) SaveCategory(ctx context.Context, c models.Category) (models.Category, error) {
	name, err := cleanName(c.Name, "nome da categoria")
	if err != nil {
		return models.Category{}, err
	}
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := requirePerm(ctx, s.Now(), PermProductCreate); err != nil {
			return err
		}
		if c.ID == 0 {
			c.ID, err = s.categories.Insert(ctx, tx, name, s.Now())
			return fromRepo(err, nil, duplicateCategoryMsg)
		}
		err := s.categories.Update(ctx, tx, c.ID, name, c.Active, s.Now())
		return fromRepo(err, newErr(CodeNotFound, "Categoria %d não encontrada.", c.ID), duplicateCategoryMsg)
	})
	if err != nil {
		return models.Category{}, err
	}
	return s.categories.GetByID(ctx, s.DB, c.ID)
}

func (s *ProductService) Get(ctx context.Context, id int64) (models.Product, error) {
	p, err := s.products.GetByID(ctx, s.DB, id)
	return hideCost(ctx, p), fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", id), "")
}

func (s *ProductService) List(ctx context.Context) ([]models.Product, error) {
	list, err := s.products.List(ctx, s.DB)
	return hideCosts(ctx, list), err
}

// SearchByBarcode busca pelo código exato.
func (s *ProductService) SearchByBarcode(ctx context.Context, code string) (models.Product, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return models.Product{}, invalid("Informe o código de barras.")
	}
	p, err := s.products.GetByBarcode(ctx, s.DB, code)
	if isNotFound(err) {
		return p, newErr(CodeProductNotFound, "Nenhum produto com o código de barras ou SKU %s.", code)
	}
	if err == nil {
		p.Promo = productPromo(ctx, s.DB, p, s.Now())
	}
	return hideCost(ctx, p), err
}

// ForSale devolve os produtos pedidos com a promoção vigente (orçamento levado
// para a tela de vendas). Os que não existem mais ficam de fora.
func (s *ProductService) ForSale(ctx context.Context, ids []int64) ([]models.Product, error) {
	out := []models.Product{}
	for _, id := range ids {
		p, err := s.products.GetByID(ctx, s.DB, id)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		p.Promo = productPromo(ctx, s.DB, p, s.Now())
		out = append(out, hideCost(ctx, p))
	}
	return out, nil
}

// Search procura por nome ou código. onlyActive é usado na tela de vendas.
func (s *ProductService) Search(ctx context.Context, query string, onlyActive bool, categoryID int64) ([]models.Product, error) {
	query = strings.TrimSpace(query)
	if query == "" && categoryID == 0 {
		return []models.Product{}, nil
	}
	list, err := s.products.Search(ctx, s.DB, query, onlyActive, categoryID, 50)
	for i := range list {
		list[i].Promo = productPromo(ctx, s.DB, list[i], s.Now())
	}
	return hideCosts(ctx, list), err
}

// maxSaleCatalog limita a lista de produtos da tela de vendas (o resto se acha pela busca).
const maxSaleCatalog = 1000

// SaleCatalog: produtos vendáveis (ativos e com preço), com a promoção vigente,
// para a lista ao lado do carrinho.
func (s *ProductService) SaleCatalog(ctx context.Context) ([]models.Product, error) {
	list, err := s.products.Search(ctx, s.DB, "", true, 0, maxSaleCatalog)
	for i := range list {
		list[i].Promo = productPromo(ctx, s.DB, list[i], s.Now())
	}
	return hideCosts(ctx, list), err
}

// canSeeCost: custo e margem só para quem tem ver_custo (o administrador sempre).
func canSeeCost(ctx context.Context) bool {
	u, ok := actor.From(ctx)
	return ok && (u.Role == models.RoleAdmin || slices.Contains(u.Perms, PermCost))
}

func hideCost(ctx context.Context, p models.Product) models.Product {
	if !canSeeCost(ctx) {
		p.CostCents = 0
	}
	return p
}

func hideCosts(ctx context.Context, ps []models.Product) []models.Product {
	for i := range ps {
		ps[i] = hideCost(ctx, ps[i])
	}
	return ps
}
