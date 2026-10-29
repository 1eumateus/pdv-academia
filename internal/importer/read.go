// Caminho: pdv/internal/importer/read.go

// Package importer lê planilhas de importação (CSV e XLSX) como uma tabela de
// textos. Não conhece produtos nem clientes: isso é do service.
package importer

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

const (
	MaxFileBytes = 10 << 20 // 10 MB
	MaxRows      = 20000
	MaxCols      = 60
)

// Read devolve as linhas do arquivo (a primeira costuma ser o cabeçalho).
// Linhas totalmente vazias são descartadas.
func Read(name string, data []byte) ([][]string, error) {
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("arquivo maior que %d MB", MaxFileBytes>>20)
	}
	var (
		rows [][]string
		err  error
	)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv", ".txt":
		rows, err = readCSV(data)
	case ".xlsx":
		rows, err = readXLSX(data)
	case ".xls":
		return nil, errors.New("arquivo .xls (Excel antigo): abra no Excel e salve como .xlsx ou .csv")
	default:
		return nil, errors.New("use um arquivo .csv ou .xlsx")
	}
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, r := range rows {
		for i := range r {
			r[i] = strings.TrimSpace(r[i])
		}
		if strings.Join(r, "") != "" {
			out = append(out, r)
		}
	}
	if len(out) > MaxRows {
		return nil, fmt.Errorf("a planilha tem mais de %d linhas; divida em arquivos menores", MaxRows)
	}
	return out, nil
}

// readCSV aceita UTF-8 (com ou sem BOM) ou Windows-1252/Latin-1 (o "CSV" do
// Excel brasileiro) e separador ";" ou "," (o que mais aparece na 1ª linha).
func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	if !utf8.Valid(data) {
		dec, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err != nil {
			return nil, errors.New("não foi possível ler o texto do arquivo")
		}
		data = dec
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ';'
	if bytes.Count(first, []byte(",")) > bytes.Count(first, []byte(";")) {
		r.Comma = ','
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV inválido: %v", err)
	}
	return rows, nil
}

// ---------- XLSX: só a primeira aba, só os valores ----------

type xlsxRel struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type xlsxCell struct {
	Ref    string `xml:"r,attr"`
	Type   string `xml:"t,attr"`
	Value  string `xml:"v"`
	Inline struct {
		Text string `xml:"t"`
		Runs []struct {
			Text string `xml:"t"`
		} `xml:"r"`
	} `xml:"is"`
}

func readXLSX(data []byte) ([][]string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("arquivo .xlsx inválido ou corrompido")
	}
	files := map[string]*zip.File{}
	for _, f := range z.File {
		files[f.Name] = f
	}
	decode := func(name string, v any) error {
		f, ok := files[name]
		if !ok {
			return fmt.Errorf("parte %s ausente", name)
		}
		if f.UncompressedSize64 > 200<<20 {
			return errors.New("planilha grande demais")
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		return xml.NewDecoder(io.LimitReader(rc, 200<<20)).Decode(v)
	}

	// Primeira aba: workbook.xml → id da relação → caminho da planilha.
	sheetPath := "xl/worksheets/sheet1.xml"
	var wb struct {
		Sheets []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	var rels struct {
		Rels []xlsxRel `xml:"Relationship"`
	}
	if decode("xl/workbook.xml", &wb) == nil && len(wb.Sheets) > 0 && decode("xl/_rels/workbook.xml.rels", &rels) == nil {
		for _, r := range rels.Rels {
			if r.ID == wb.Sheets[0].RID {
				if strings.HasPrefix(r.Target, "/") {
					sheetPath = strings.TrimPrefix(r.Target, "/")
				} else {
					sheetPath = path.Join("xl", r.Target)
				}
			}
		}
	}

	var shared []string
	if _, ok := files["xl/sharedStrings.xml"]; ok {
		var sst struct {
			Items []struct {
				Text string `xml:"t"`
				Runs []struct {
					Text string `xml:"t"`
				} `xml:"r"`
			} `xml:"si"`
		}
		if err := decode("xl/sharedStrings.xml", &sst); err != nil {
			return nil, errors.New("arquivo .xlsx inválido (textos)")
		}
		for _, it := range sst.Items {
			s := it.Text
			for _, r := range it.Runs {
				s += r.Text
			}
			shared = append(shared, s)
		}
	}

	var ws struct {
		Rows []struct {
			Cells []xlsxCell `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := decode(sheetPath, &ws); err != nil {
		return nil, errors.New("arquivo .xlsx inválido (planilha)")
	}
	out := make([][]string, 0, len(ws.Rows))
	for _, row := range ws.Rows {
		var r []string
		for i, c := range row.Cells {
			col := i
			if c.Ref != "" {
				col = colIndex(c.Ref)
			}
			if col < 0 || col >= MaxCols {
				continue
			}
			for len(r) <= col {
				r = append(r, "")
			}
			switch c.Type {
			case "s":
				if n, err := strconv.Atoi(c.Value); err == nil && n >= 0 && n < len(shared) {
					r[col] = shared[n]
				}
			case "inlineStr":
				r[col] = c.Inline.Text
				for _, run := range c.Inline.Runs {
					r[col] += run.Text
				}
			case "b":
				r[col] = map[string]string{"1": "sim", "0": "não"}[c.Value]
			default: // n, str, e
				r[col] = c.Value
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// colIndex: "C12" → 2.
func colIndex(ref string) int {
	n := 0
	for _, ch := range ref {
		if ch < 'A' || ch > 'Z' {
			break
		}
		n = n*26 + int(ch-'A'+1)
	}
	return n - 1
}
