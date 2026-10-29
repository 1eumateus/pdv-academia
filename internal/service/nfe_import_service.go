// Caminho: pdv/internal/service/nfe_import_service.go

package service

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pdv/internal/models"
	"pdv/internal/nfe"
	"pdv/internal/repository"
)

// NFeService importa o XML da NF-e de compra: confere (sem gravar) e confirma
// (fornecedor, vínculos e compra na mesma transação). O XML é sempre relido do
// arquivo no backend; da tela só vêm as escolhas do usuário.
type NFeService struct {
	Base
	dataDir   string
	settings  *SettingsService
	purchases *PurchaseService
	suppliers repository.SupplierRepo
	products  repository.ProductRepo
}

func NewNFeService(b Base, dataDir string) *NFeService {
	return &NFeService{Base: b, dataDir: dataDir, settings: NewSettingsService(b), purchases: NewPurchaseService(b)}
}

func readNFe(path string) (*nfe.Invoice, []byte, error) {
	if !strings.EqualFold(filepath.Ext(path), ".xml") {
		return nil, nil, invalid("Escolha um arquivo .xml da NF-e.")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, invalid("Não foi possível ler o arquivo %s.", filepath.Base(path))
	}
	inv, err := nfe.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, nil, invalid("%s: %v.", filepath.Base(path), err)
	}
	return inv, data, nil
}

// itemCost soma ao valor dos produtos o que estiver ligado em Configurações.
func itemCost(it nfe.Item, st models.Settings) int64 {
	c := it.ProductsCents
	if st.NFeCostFreight {
		c += it.FreightCents
	}
	if st.NFeCostIPI {
		c += it.IPICents
	}
	if st.NFeCostST {
		c += it.STCents
	}
	if st.NFeCostOther {
		c += it.InsuranceAndOtherCents
	}
	if st.NFeCostDiscount {
		c -= it.DiscountCents
	}
	return max(c, 0)
}

func qtyText(q4 int64) string {
	s := fmt.Sprintf("%d,%04d", q4/nfe.QtyScale, q4%nfe.QtyScale)
	return strings.TrimSuffix(strings.TrimRight(s, "0"), ",")
}

// fill calcula unidades e custo unitário de um item com o produto e o fator escolhidos.
func fill(item *models.NFeItem, q4 int64) {
	item.Units, item.UnitCostCents, item.Problem = 0, 0, ""
	if item.Factor < 1 {
		item.Factor = 1
	}
	total4 := q4 * item.Factor
	if total4%nfe.QtyScale != 0 {
		item.Problem = fmt.Sprintf("%s %s × %d não dá um número inteiro de unidades; o sistema só vende unidades inteiras. Ajuste o fator.",
			item.QtyText, item.Unit, item.Factor)
		return
	}
	item.Units = total4 / nfe.QtyScale
	if item.Units > MaxQty {
		item.Units, item.Problem = 0, "Quantidade acima do limite do sistema."
		return
	}
	item.UnitCostCents = (item.CostCents + item.Units/2) / item.Units
	if item.ProductID == nil {
		item.Problem = "Escolha o produto do sistema ou cadastre um novo."
	}
}

// preview monta a conferência usando só q (dentro da transação na confirmação:
// o banco tem uma conexão, então nada aqui pode usar s.DB). Configurações vêm prontas.
func (s *NFeService) preview(ctx context.Context, q repository.DBTX, inv *nfe.Invoice, st models.Settings) (models.NFePreview, error) {
	var err error
	out := models.NFePreview{AccessKey: inv.AccessKey, Number: inv.Number, Series: inv.Series, IssueDate: inv.IssueDate,
		InvoiceCents: inv.Totals.InvoiceCents}
	if out.AlreadyImported, err = s.purchases.purchases.ExistsAccessKey(ctx, q, inv.AccessKey); err != nil {
		return out, err
	}
	sup, err := s.suppliers.GetByDocument(ctx, q, inv.Issuer.Document)
	switch {
	case err == nil:
		out.Supplier = sup
	case isNotFound(err):
		a := inv.Issuer.Address
		out.Supplier = models.Supplier{LegalName: inv.Issuer.LegalName, TradeName: inv.Issuer.TradeName,
			Document: inv.Issuer.Document, StateRegistration: inv.Issuer.StateRegistration, Phone: a.Phone,
			ZipCode: a.ZipCode, Street: a.Street, Number: a.Number, Complement: a.Complement, District: a.District,
			City: a.City, State: a.State, Active: true}
	default:
		return out, err
	}
	for _, it := range inv.Items {
		item := models.NFeItem{Number: it.Number, Code: it.Code, EAN: it.EAN, Name: it.Name, NCM: it.NCM, Unit: it.Unit,
			QtyText: qtyText(it.Qty4), LineCents: it.ProductsCents, CostCents: itemCost(it, st), Factor: 1}
		var pid int64
		if it.EAN != "" {
			if p, err := s.products.GetByBarcode(ctx, q, it.EAN); err == nil {
				pid, item.MatchedBy = p.ID, "codigo_barras"
			} else if !isNotFound(err) {
				return out, err
			}
		}
		if out.Supplier.ID != 0 && it.Code != "" {
			lp, factor, err := s.suppliers.Link(ctx, q, out.Supplier.ID, it.Code)
			switch {
			case err == nil:
				item.Factor = factor
				if pid == 0 {
					pid, item.MatchedBy = lp, "vinculo"
				}
			case !isNotFound(err):
				return out, err
			}
		}
		if pid != 0 {
			p, err := s.products.GetByID(ctx, q, pid)
			if err != nil {
				return out, err
			}
			p = hideCost(ctx, p)
			item.ProductID, item.ProductName, item.PriceCents, item.CurrentCost = &p.ID, &p.Name, p.PriceCents, p.CostCents
			item.ProductCode = p.Barcode
		}
		fill(&item, it.Qty4)
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// PreviewFile confere a nota sem gravar nada.
func (s *NFeService) PreviewFile(ctx context.Context, path string) (models.NFePreview, error) {
	if _, err := requirePerm(ctx, s.Now(), PermPurchases); err != nil {
		return models.NFePreview{}, err
	}
	inv, _, err := readNFe(path)
	if err != nil {
		return models.NFePreview{}, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return models.NFePreview{}, err
	}
	return s.preview(ctx, s.DB, inv, st)
}

// applyDecision cadastra o produto novo (ProductID 0 com nome) ou renomeia o
// escolhido quando o nome/código mudou. Devolve o id do produto (0 = nenhum).
func (s *NFeService) applyDecision(ctx context.Context, tx *sql.Tx, d models.NFeDecision, priceLater bool, now time.Time) (int64, error) {
	name := strings.TrimSpace(d.Name)
	code := trimPtr(&d.Code)
	if d.ProductID == 0 {
		if name == "" {
			return 0, nil
		}
		if _, err := requirePerm(ctx, now, PermProductCreate); err != nil {
			return 0, err
		}
		var price int64
		switch {
		case d.NewPriceCents != nil:
			price = *d.NewPriceCents
		case !priceLater:
			return 0, invalid("informe o preço de venda do produto novo (ou marque \"precificar depois\")")
		}
		f, err := validateProduct(models.ProductInput{Name: name, Barcode: code, PriceCents: price, Active: true})
		if err != nil {
			return 0, err
		}
		f.PricePending = d.NewPriceCents == nil
		id, err := s.products.Insert(ctx, tx, f, now)
		return id, fromRepo(err, nil, duplicateBarcodeMsg(f))
	}
	p, err := s.products.GetByID(ctx, tx, d.ProductID)
	if err != nil {
		return 0, fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", d.ProductID), "")
	}
	if name == "" || (name == p.Name && str(code) == str(p.Barcode)) {
		return p.ID, nil
	}
	n, err := cleanName(name, "nome do produto")
	if err != nil {
		return 0, err
	}
	bc, err := cleanBarcode(code)
	if err != nil {
		return 0, err
	}
	f := repository.ProductFields{Name: n, Barcode: bc, Sku: p.Sku, PriceCents: p.PriceCents,
		Active: p.Active, CategoryID: p.CategoryID, CostCents: p.CostCents, MinStock: p.MinStock}
	err = s.products.Update(ctx, tx, p.ID, f, now)
	return p.ID, fromRepo(err, nil, duplicateBarcodeMsg(f))
}

// ConfirmFile relê o XML, aplica as escolhas e grava fornecedor (se novo),
// vínculos e a compra numa única transação. Depois guarda o XML original.
// priceLater: produtos novos sem preço são cadastrados "a precificar" (fora da venda até ter preço).
func (s *NFeService) ConfirmFile(ctx context.Context, path string, decisions []models.NFeDecision, priceLater bool) (models.PurchaseDetails, error) {
	inv, raw, err := readNFe(path)
	if err != nil {
		return models.PurchaseDetails{}, err
	}
	st, err := s.settings.Get(ctx)
	if err != nil {
		return models.PurchaseDetails{}, err
	}
	byNumber := map[int]models.NFeDecision{}
	for _, d := range decisions {
		byNumber[d.Number] = d
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := requirePerm(ctx, now, PermPurchases); err != nil {
			return err
		}
		pv, err := s.preview(ctx, tx, inv, st)
		if err != nil {
			return err
		}
		if pv.AlreadyImported {
			return newErr(CodeDuplicate, "Esta nota (nº %s, chave %s) já foi importada.", pv.Number, pv.AccessKey)
		}
		sup := pv.Supplier
		if sup.ID == 0 {
			clean, err := validateSupplier(sup)
			if err != nil {
				return invalid("Dados do fornecedor na nota: %v", err)
			}
			clean.Active = true
			if sup.ID, err = s.suppliers.Insert(ctx, tx, clean, now); err != nil {
				return fromRepo(err, nil, duplicateSupplierMsg)
			}
			if err := audit(ctx, tx, now, nil, "fornecedor_cadastrado", "fornecedor", &sup.ID, nil,
				map[string]any{"razao_social": clean.LegalName, "documento": clean.Document, "origem": "XML da NF-e"}); err != nil {
				return err
			}
		} else if !sup.Active {
			return invalid("O fornecedor %s está desativado. Ative-o em Fornecedores para importar a nota.", sup.LegalName)
		}

		// Aplica as escolhas e junta itens da nota que caíram no mesmo produto.
		type acc struct {
			units, cost int64
			newPrice    *int64
		}
		merged := map[int64]*acc{}
		var order []int64
		var skipped []string
		for i, it := range pv.Items {
			d, ok := byNumber[it.Number]
			if ok && d.Skip {
				skipped = append(skipped, strconv.Itoa(it.Number))
				continue
			}
			if ok {
				pid, err := s.applyDecision(ctx, tx, d, priceLater, now)
				if err != nil {
					return invalid("Item %d (%s): %v", it.Number, it.Name, err)
				}
				if pid != 0 {
					it.ProductID = &pid
				}
				it.Factor = d.Factor
			}
			fill(&it, inv.Items[i].Qty4)
			if it.Problem != "" {
				return invalid("Item %d (%s): %s", it.Number, it.Name, it.Problem)
			}
			pid := *it.ProductID
			if merged[pid] == nil {
				merged[pid] = &acc{}
				order = append(order, pid)
			}
			m := merged[pid]
			m.units += it.Units
			m.cost += it.CostCents
			if ok && d.NewPriceCents != nil {
				m.newPrice = d.NewPriceCents
			}
			if it.Code != "" {
				if err := s.suppliers.SaveLink(ctx, tx, sup.ID, it.Code, pid, it.Factor, now); err != nil {
					return err
				}
			}
		}
		if len(order) == 0 {
			return invalid("Todos os itens foram marcados como não recebidos. Mantenha ao menos um para lançar a entrada.")
		}
		obs := fmt.Sprintf("Importada do XML da NF-e série %s", pv.Series)
		if len(skipped) > 0 {
			obs += ". Itens da nota não recebidos: " + strings.Join(skipped, ", ")
		}
		in := models.PurchaseInput{SupplierID: sup.ID, InvoiceNumber: pv.Number, IssueDate: pv.IssueDate, AccessKey: pv.AccessKey,
			FreightCents: inv.Totals.FreightCents, TaxesCents: inv.Totals.IPICents + inv.Totals.STCents,
			DiscountCents: inv.Totals.DiscountCents, Notes: obs}
		for _, pid := range order {
			m := merged[pid]
			in.Items = append(in.Items, models.PurchaseItemInput{ProductID: pid, Qty: m.units,
				UnitCostCents: (m.cost + m.units/2) / m.units, NewPriceCents: m.newPrice})
		}
		invoice, notes, err := validatePurchase(in)
		if err != nil {
			return err
		}
		id, err = s.purchases.createTx(ctx, tx, in, invoice, notes)
		return err
	})
	if err != nil {
		return models.PurchaseDetails{}, err
	}
	// Guardar o XML é complementar: a compra já está gravada mesmo se falhar.
	dir := filepath.Join(s.dataDir, "nfe")
	if err := os.MkdirAll(dir, 0o700); err == nil {
		_ = os.WriteFile(filepath.Join(dir, inv.AccessKey+".xml"), raw, 0o600)
	}
	return s.purchases.Details(ctx, id)
}
