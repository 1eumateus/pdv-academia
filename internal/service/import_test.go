// Caminho: pdv/internal/service/import_test.go

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pdv/internal/models"
)

func writeCSV(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "planilha.csv")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportProducts(t *testing.T) {
	e := newEnv(t)
	is := NewImportService(e.products.Base)
	bc := "7890000000017"
	old, _ := e.products.Create(e.ctx, models.ProductInput{Name: "Feijão", Barcode: &bc, PriceCents: 800, Active: true, InitialStock: 5})

	path := writeCSV(t, "Descrição;Código de barras;Preço;Custo;Estoque;Categoria\n"+
		"Arroz 5kg;7890000000024;R$ 27,90;21,5;40;Mercearia\n"+
		"Feijão carioca;7890000000017;9,50;;12;\n"+
		"Sem preço;;;;;\n"+
		"Repetido;7890000000024;1;;;\n")
	f, err := is.Open(e.ctx, "produtos", path)
	if err != nil {
		t.Fatal(err)
	}
	m := f.Mapping
	if m["nome"] != 0 || m["codigo_barras"] != 1 || m["preco"] != 2 || m["custo"] != 3 || m["estoque"] != 4 || m["categoria"] != 5 {
		t.Fatalf("mapeamento sugerido: %v", m)
	}
	req := models.ImportRequest{Kind: "produtos", Path: path, Mapping: m, Update: true}
	prev, err := is.Preview(e.ctx, req)
	if err != nil || prev.New != 1 || prev.Updated != 1 || prev.Errors != 2 {
		t.Fatalf("prévia: %+v %v", prev, err)
	}
	if !strings.Contains(prev.Rows[3].Message, "repetido (linha 2)") || !strings.Contains(prev.Rows[2].Message, "Preço de venda em branco") {
		t.Fatalf("mensagens: %+v", prev.Rows)
	}
	// Com erro, nada é gravado.
	if _, err := is.Confirm(e.ctx, req); err == nil {
		t.Fatal("gravou com linhas com erro")
	}
	if e.stockOf(old.ID) != 5 {
		t.Fatal("alterou o banco mesmo recusando")
	}

	// Planilha corrigida: cria, atualiza, cria categoria, ajusta estoque.
	path = writeCSV(t, "Descrição;Código de barras;Preço;Custo;Estoque;Categoria\n"+
		"Arroz 5kg;7890000000024;R$ 27,90;21,5;40;Mercearia\n"+
		"Feijão carioca;7890000000017;9,50;;12;\n")
	req.Path = path
	if prev, err = is.Confirm(e.ctx, req); err != nil || prev.New != 1 || prev.Updated != 1 {
		t.Fatalf("importação: %+v %v", prev, err)
	}
	arroz, err := e.products.SearchByBarcode(e.ctx, "7890000000024")
	if err != nil || arroz.PriceCents != 2790 || arroz.CostCents != 2150 || arroz.StockQty != 40 || arroz.CategoryName == nil || *arroz.CategoryName != "Mercearia" {
		t.Fatalf("novo: %+v %v", arroz, err)
	}
	feijao, _ := e.products.SearchByBarcode(e.ctx, bc)
	if feijao.Name != "Feijão carioca" || feijao.PriceCents != 950 || e.stockOf(old.ID) != 12 {
		t.Fatalf("atualizado: %+v", feijao)
	}
	// Sem "atualizar", os já cadastrados são ignorados.
	req.Update = false
	if prev, _ = is.Preview(e.ctx, req); prev.Skipped != 2 || prev.New != 0 {
		t.Fatalf("só criar: %+v", prev)
	}
}

func TestImportCustomersAndTemplate(t *testing.T) {
	e := newEnv(t)
	is := NewImportService(e.products.Base)
	path := writeCSV(t, "Nome,CPF,Telefone,UF,Limite\nAna,529.982.247-25,11999990000,sp,\"150,00\"\nBruno,111.111.111-11,,,\n")
	f, _ := is.Open(e.ctx, "clientes", path)
	req := models.ImportRequest{Kind: "clientes", Path: path, Mapping: f.Mapping}
	prev, err := is.Preview(e.ctx, req)
	if err != nil || prev.New != 1 || prev.Errors != 1 || !strings.Contains(prev.Rows[1].Message, "CPF inválido") {
		t.Fatalf("prévia clientes: %+v %v", prev, err)
	}
	path = writeCSV(t, "Nome,CPF,Telefone,UF,Limite\nAna,529.982.247-25,11999990000,sp,\"150,00\"\n")
	req.Path = path
	if _, err := is.Confirm(e.ctx, req); err != nil {
		t.Fatal(err)
	}
	list, _ := e.customers.List(e.ctx, "Ana")
	if len(list) != 1 || list[0].CreditLimitCents != 15000 || *list[0].State != "SP" || *list[0].Document != "52998224725" {
		t.Fatalf("cliente: %+v", list)
	}

	// O modelo para baixar é lido de volta com o mapeamento certo e os códigos reservados.
	codes, err := e.products.ReserveCodes(e.ctx, 3)
	if err != nil || len(codes) != 3 {
		t.Fatal(codes, err)
	}
	data, err := ImportTemplate("produtos", codes)
	if err != nil {
		t.Fatal(err)
	}
	tpl := filepath.Join(t.TempDir(), "modelo.xlsx")
	os.WriteFile(tpl, data, 0o600)
	f, err = is.Open(e.ctx, "produtos", tpl)
	if err != nil || len(f.Mapping) != 8 || f.Rows != 3 || f.Sample[0][1] != "" || f.Sample[0][2] != codes[0].Sku {
		t.Fatalf("modelo: %+v %v", f, err)
	}
	// Linhas só com o código reservado não contam; nada a importar.
	if prev, err := is.Preview(e.ctx, models.ImportRequest{Kind: "produtos", Path: tpl, Mapping: f.Mapping}); err != nil || len(prev.Rows) != 0 {
		t.Fatalf("modelo vazio: %+v %v", prev, err)
	}
	// "(opcional)" nos cabeçalhos do modelo não atrapalha o reconhecimento (nem do "UF").
	data, _ = ImportTemplate("clientes", nil)
	tplc := filepath.Join(t.TempDir(), "clientes.xlsx")
	os.WriteFile(tplc, data, 0o600)
	if fc, err := is.Open(e.ctx, "clientes", tplc); err != nil || len(fc.Mapping) != 12 {
		t.Fatalf("modelo de clientes: %+v %v", fc.Mapping, err)
	}
}

func TestImportStockBecomesPurchase(t *testing.T) {
	e := newEnv(t)
	is := NewImportService(e.products.Base)
	path := writeCSV(t, "Descrição;Código de barras;Preço;Custo;Estoque\n"+
		"Arroz 5kg;7890000000024;27,90;21,50;40\n"+
		"Sal;7890000000031;3,00;;\n"+
		";2000000000015;;;\n")
	f, _ := is.Open(e.ctx, "produtos", path)
	if _, err := is.Confirm(e.ctx, models.ImportRequest{Kind: "produtos", Path: path, Mapping: f.Mapping}); err != nil {
		t.Fatal(err)
	}
	ps := NewPurchaseService(e.products.Base)
	day := e.products.Now().Format("2006-01-02")
	list, err := ps.List(e.ctx, day, day, "")
	if err != nil || len(list) != 1 || list[0].SupplierName != noSupplierName || list[0].TotalCents != 40*2150 {
		t.Fatalf("entrada: %+v %v", list, err)
	}
	d, _ := ps.Details(e.ctx, list[0].ID)
	if len(d.Items) != 1 || d.Items[0].Qty != 40 {
		t.Fatalf("itens: %+v", d.Items)
	}
	arroz, _ := e.products.SearchByBarcode(e.ctx, "7890000000024")
	if arroz.StockQty != 40 || arroz.CostCents != 2150 {
		t.Fatalf("arroz: %+v", arroz)
	}
	// Estornar a entrada tira o estoque que a planilha colocou.
	if _, err := ps.Cancel(e.ctx, list[0].ID, "planilha errada"); err != nil || e.stockOf(arroz.ID) != 0 {
		t.Fatal("estorno", err)
	}
}
