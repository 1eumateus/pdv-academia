// Caminho: pdv/internal/nfe/nfe_test.go

package nfe

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func parseFile(t *testing.T, name string) *Invoice {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	inv, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestParseNfeProc(t *testing.T) {
	inv := parseFile(t, "nfeproc_compra.xml")
	if inv.AccessKey != "35261011222333000181550010000012341000012345" || inv.Number != "1234" || inv.Series != "1" ||
		inv.IssueDate != "2026-10-10" {
		t.Fatalf("cabeçalho: %+v", inv)
	}
	is := inv.Issuer
	if is.Document != "11222333000181" || is.LegalName != "DISTRIBUIDORA EXEMPLO LTDA" || is.TradeName != "EXEMPLO DIST" ||
		is.StateRegistration != "123456789110" || is.Address.City != "SAO PAULO" || is.Address.ZipCode != "01001000" {
		t.Fatalf("emitente: %+v", is)
	}
	if len(inv.Items) != 2 {
		t.Fatalf("itens: %d", len(inv.Items))
	}
	a, b := inv.Items[0], inv.Items[1]
	if a.Code != "CAN-AZ" || a.EAN != "7891234567895" || a.Unit != "UN" || a.Qty4 != 10*QtyScale ||
		a.ProductsCents != 2500 || a.FreightCents != 200 || a.IPICents != 100 || a.STCents != 0 || a.NCM != "96081000" {
		t.Fatalf("item 1: %+v", a)
	}
	if b.EAN != "" || b.Unit != "CX" || b.Qty4 != 2*QtyScale || b.ProductsCents != 24000 || b.DiscountCents != 1000 ||
		b.STCents != 1234 || b.IPICents != 0 {
		t.Fatalf("item 2 (SEM GTIN, caixa, ST): %+v", b)
	}
	if inv.Totals.InvoiceCents != 27834 || inv.Totals.FreightCents != 1000 || inv.Totals.STCents != 1234 {
		t.Fatalf("totais: %+v", inv.Totals)
	}
}

func TestParseNFeWithoutProtocol(t *testing.T) {
	data, _ := os.ReadFile("testdata/nfeproc_compra.xml")
	s := string(data)
	// Só o elemento <NFe>, sem o envelope de autorização: a chave vem do Id.
	nfe := s[strings.Index(s, "<NFe ") : strings.Index(s, "</NFe>")+len("</NFe>")]
	inv, err := Parse(strings.NewReader(nfe))
	if err != nil || inv.AccessKey != "35261011222333000181550010000012341000012345" {
		t.Fatalf("NFe sem protocolo: %+v %v", inv, err)
	}
}

func TestParseRejects(t *testing.T) {
	data, _ := os.ReadFile("testdata/nfeproc_compra.xml")
	other := strings.Replace(string(data), "<mod>55</mod>", "<mod>57</mod>", 1)
	cases := map[string]string{
		"lixo":      "isto não é xml",
		"outro":     `<?xml version="1.0"?><pedido><n>1</n></pedido>`,
		"modelo 57": other,
		"vazio":     "",
		"qtd":       strings.Replace(string(data), "<qCom>10.0000</qCom>", "<qCom>abc</qCom>", 1),
		"valor":     strings.Replace(string(data), "<vProd>25.00</vProd>", "<vProd>25,00</vProd>", 1),
	}
	for name, xml := range cases {
		if _, err := Parse(strings.NewReader(xml)); err == nil {
			t.Errorf("%s: deveria recusar", name)
		}
	}
	if _, err := Parse(strings.NewReader(other)); err == nil || !strings.Contains(err.Error(), "modelo 57") {
		t.Fatalf("mensagem do modelo: %v", err)
	}
	// NFC-e (modelo 65) tem a mesma estrutura e é aceita.
	nfce := strings.Replace(string(data), "<mod>55</mod>", "<mod>65</mod>", 1)
	if inv, err := Parse(strings.NewReader(nfce)); err != nil || len(inv.Items) != 2 {
		t.Fatalf("NFC-e deveria ser lida: %v", err)
	}
}

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		in     string
		places int
		want   int64
		bad    bool
	}{
		{"25.00", 2, 2500, false}, {"25", 2, 2500, false}, {"0.005", 2, 1, false}, {"0.004", 2, 0, false},
		{"2.5000000000", 2, 250, false}, {"10.0000", 4, 100000, false}, {"1.23456", 4, 12346, false},
		{"", 2, 0, false}, {"-1.00", 2, 0, true}, {"1,00", 2, 0, true}, {"1.2.3", 2, 0, true},
	}
	for _, c := range cases {
		got, err := ParseDecimal(c.in, c.places)
		if (err != nil) != c.bad || (!c.bad && got != c.want) {
			t.Errorf("ParseDecimal(%q, %d) = %d, %v", c.in, c.places, got, err)
		}
	}
}

// Notas reais às vezes vêm "formatadas", com quebras de linha e espaços dentro de cada valor.
func TestParsePrettyPrinted(t *testing.T) {
	data, _ := os.ReadFile("testdata/nfeproc_compra.xml")
	pretty := regexp.MustCompile(`>([^<>]+)</`).ReplaceAllString(string(data), ">\n                    $1\n                </")
	inv, err := Parse(strings.NewReader(pretty))
	if err != nil {
		t.Fatal(err)
	}
	it := inv.Items[0]
	if inv.IssueDate != "2026-10-10" || inv.Number != "1234" || it.EAN != "7891234567895" || it.Code != "CAN-AZ" ||
		it.Unit != "UN" || it.Qty4 != 10*QtyScale || it.ProductsCents != 2500 || inv.Issuer.LegalName != "DISTRIBUIDORA EXEMPLO LTDA" {
		t.Fatalf("XML formatado lido errado: %+v / %+v", inv, it)
	}
}
