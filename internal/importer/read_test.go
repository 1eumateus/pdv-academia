// Caminho: pdv/internal/importer/read_test.go

package importer

import (
	"archive/zip"
	"bytes"
	"reflect"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestReadCSVLatin1Semicolon(t *testing.T) {
	latin, _ := charmap.Windows1252.NewEncoder().String("Nome;Preço\nPão de açúcar;\"4,50\"\n;\nCafé;12\n")
	rows, err := Read("x.csv", []byte(latin))
	want := [][]string{{"Nome", "Preço"}, {"Pão de açúcar", "4,50"}, {"Café", "12"}}
	if err != nil || !reflect.DeepEqual(rows, want) {
		t.Fatalf("%v %v", rows, err)
	}
	// UTF-8 com BOM e vírgula.
	rows, err = Read("x.csv", []byte("\xEF\xBB\xBFnome,preco\nÁgua,\"2,00\"\n"))
	if err != nil || rows[1][0] != "Água" || rows[1][1] != "2,00" || rows[0][0] != "nome" {
		t.Fatalf("utf8: %v %v", rows, err)
	}
}

// Planilha como o Excel grava: textos em sharedStrings, aba em sheet2.xml,
// célula vazia pulada (B2 ausente).
func TestReadXLSXSharedStrings(t *testing.T) {
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Plan1" sheetId="1" r:id="rId7"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId7" Target="worksheets/sheet2.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><si><t>Nome</t></si><si><t>Preço</t></si><si><r><t>Arroz </t></r><r><t>5kg</t></r></si></sst>`,
		"xl/worksheets/sheet2.xml": `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>
<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>Estoque</t></is></c></row>
<row r="2"><c r="A2" t="s"><v>2</v></c><c r="C2"><v>10</v></c></row>
<row r="3"><c r="B3"><v>25.9</v></c></row></sheetData></worksheet>`,
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range parts {
		w, _ := z.Create(name)
		w.Write([]byte(body))
	}
	z.Close()
	rows, err := Read("p.xlsx", buf.Bytes())
	want := [][]string{{"Nome", "Preço", "Estoque"}, {"Arroz 5kg", "", "10"}, {"", "25.9"}}
	if err != nil || !reflect.DeepEqual(rows, want) {
		t.Fatalf("%q %v", rows, err)
	}
	if _, err := Read("p.xls", nil); err == nil {
		t.Fatal("aceitou .xls")
	}
}
