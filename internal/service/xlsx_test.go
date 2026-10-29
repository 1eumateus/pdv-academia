// Caminho: pdv/internal/service/xlsx_test.go

package service

import (
	"pdv/internal/models"

	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

// Todas as partes do .xlsx precisam ser XML válido, senão o Excel diz que o arquivo está corrompido.
func xlsxParts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range z.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		d := xml.NewDecoder(bytes.NewReader(b))
		for {
			if _, err := d.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("%s: XML inválido: %v", f.Name, err)
			}
		}
		out[f.Name] = string(b)
	}
	return out
}

func TestWriteXLSXLayout(t *testing.T) {
	rows := [][]xlsxCell{
		{{text: "Lucro & margem", style: xsTitle}},
		{{text: "out/2026", style: xsMuted}},
		{},
		{{text: "Produto", style: xsHeader}, {text: "Total", style: xsHeaderRight}},
		{{text: "Uma descrição de produto bem comprida"}, {num: "12.50", style: xsMoney}},
		{{}, {num: "1.00", style: xsMoney}},
	}
	data, err := writeXLSX("Lucro & margem", []float64{10, 10}, []string{"A1:B1"}, rows, false)
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("PDV_XLSX_OUT"); dir != "" { // para abrir no Excel e conferir
		_ = os.WriteFile(dir+"/exemplo.xlsx", data, 0o644)
	}
	p := xlsxParts(t, data)
	sheet := p["xl/worksheets/sheet1.xml"]
	for _, want := range []string{`ySplit="4"`, `<autoFilter ref="A4:B6"/>`, `<c r="A6" s="1"/>`, `width="40"`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("planilha sem %s", want)
		}
	}
	if !strings.Contains(p["xl/workbook.xml"], `&#39;Lucro &amp; margem&#39;!$4:$4`) {
		t.Errorf("cabeçalho não repete na impressão: %s", p["xl/workbook.xml"])
	}
}

func TestImportTemplateXLSXValid(t *testing.T) {
	for _, kind := range []string{importProducts, importCustomers} {
		data, err := ImportTemplate(kind, []models.ProductCode{seqCode(1)})
		if err != nil {
			t.Fatal(err)
		}
		sheet := xlsxParts(t, data)["xl/worksheets/sheet1.xml"]
		if !strings.Contains(sheet, `showGridLines="1"`) || strings.Contains(sheet, "autoFilter") {
			t.Errorf("%s: modelo deve ter grade e não ter filtro", kind)
		}
	}
}
