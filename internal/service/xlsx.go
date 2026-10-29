// Caminho: pdv/internal/service/xlsx.go

package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
)

// Planilha .xlsx mínima (uma aba, até 26 colunas) escrita à mão, para não
// trazer uma biblioteca inteira só para exportar relatórios.
// shortcut: sem fórmulas nem várias abas; se precisar, trocar por excelize.

// Estilos: o número no arquivo é a posição + 1, porque o 1º xf de cellXfs é o
// padrão do Excel para células vazias e precisa ficar sem borda.
const (
	xsText = iota
	xsTitle
	xsHeader
	xsHeaderRight
	xsMoney
	xsMoneyBold
	xsInt
	xsBold
	xsMuted
	xsPct
)

// xlsxCell: num preenchido = número (com ponto decimal); senão, texto.
type xlsxCell struct {
	text  string
	num   string
	style int
}

const xlsxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<numFmts count="1"><numFmt numFmtId="164" formatCode="&quot;R$&quot;\ #,##0.00;\-&quot;R$&quot;\ #,##0.00"/></numFmts>
<fonts count="5">
<font><sz val="11"/><name val="Calibri"/></font>
<font><b/><sz val="11"/><name val="Calibri"/></font>
<font><b/><sz val="16"/><color rgb="FF1B437F"/><name val="Calibri"/></font>
<font><sz val="10"/><color rgb="FF6B7280"/><name val="Calibri"/></font>
<font><b/><sz val="11"/><color rgb="FFFFFFFF"/><name val="Calibri"/></font>
</fonts>
<fills count="3">
<fill><patternFill patternType="none"/></fill>
<fill><patternFill patternType="gray125"/></fill>
<fill><patternFill patternType="solid"><fgColor rgb="FF1B437F"/><bgColor indexed="64"/></patternFill></fill>
</fills>
<borders count="3">
<border><left/><right/><top/><bottom/><diagonal/></border>
<border><left style="thin"><color rgb="FFC9D3E0"/></left><right style="thin"><color rgb="FFC9D3E0"/></right><top style="thin"><color rgb="FFC9D3E0"/></top><bottom style="thin"><color rgb="FFC9D3E0"/></bottom><diagonal/></border>
<border><left style="thin"><color rgb="FF1B437F"/></left><right style="thin"><color rgb="FF1B437F"/></right><top style="thin"><color rgb="FF1B437F"/></top><bottom style="thin"><color rgb="FF1B437F"/></bottom><diagonal/></border>
</borders>
<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
<cellXfs count="11">
<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
<xf numFmtId="0" fontId="0" fillId="0" borderId="1" xfId="0" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="center" indent="1"/></xf>
<xf numFmtId="0" fontId="2" fillId="0" borderId="0" xfId="0" applyFont="1" applyAlignment="1"><alignment vertical="center"/></xf>
<xf numFmtId="0" fontId="4" fillId="2" borderId="2" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="center" wrapText="1" indent="1"/></xf>
<xf numFmtId="0" fontId="4" fillId="2" borderId="2" xfId="0" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center" wrapText="1" indent="1"/></xf>
<xf numFmtId="164" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center" indent="1"/></xf>
<xf numFmtId="164" fontId="1" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyFont="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center" indent="1"/></xf>
<xf numFmtId="3" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center" indent="1"/></xf>
<xf numFmtId="0" fontId="1" fillId="0" borderId="1" xfId="0" applyFont="1" applyBorder="1" applyAlignment="1"><alignment horizontal="left" vertical="center" indent="1"/></xf>
<xf numFmtId="0" fontId="3" fillId="0" borderId="0" xfId="0" applyFont="1"/>
<xf numFmtId="10" fontId="0" fillId="0" borderId="1" xfId="0" applyNumberFormat="1" applyBorder="1" applyAlignment="1"><alignment horizontal="right" vertical="center" indent="1"/></xf>
</cellXfs>
<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`

func xmlText(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// writeXLSX monta o arquivo. merges: intervalos como "A1:C1". grid: mostra as
// linhas de grade (modelo para preencher). widths são o mínimo: a coluna cresce
// com o conteúdo. Com uma única linha de cabeçalho (listagens e modelo), ela fica
// congelada, repete em cada página impressa e, havendo dados, ganha filtro.
func writeXLSX(sheet string, widths []float64, merges []string, rows [][]xlsxCell, grid bool) ([]byte, error) {
	ncols := len(widths)
	headerRow := -1 // índice da linha de cabeçalho, se houver só uma
	for r, row := range rows {
		if len(row) > ncols {
			ncols = len(row)
		}
		if len(row) > 0 && (row[0].style == xsHeader || row[0].style == xsHeaderRight) {
			if headerRow == -1 {
				headerRow = r
			} else {
				headerRow = -2
			}
		}
	}
	cols := make([]float64, ncols)
	copy(cols, widths)
	for _, row := range rows {
		for c, cell := range row {
			if cell.style == xsTitle || cell.style == xsMuted {
				continue // ficam mescladas ou transbordam; não alargam a coluna
			}
			w := float64(len([]rune(cell.text))) + 3
			if cell.num != "" {
				w = float64(len(cell.num)) + 6 // "R$ " e separador de milhar
			}
			if cell.style == xsHeader || cell.style == xsHeaderRight {
				w = min(w, 24) // cabeçalho quebra linha
			}
			cols[c] = max(cols[c], min(w, 60))
		}
	}
	lastCol := string(rune('A' + ncols - 1))

	showGrid := "0"
	if grid {
		showGrid = "1"
	}
	orientation := "portrait"
	if ncols > 5 {
		orientation = "landscape"
	}
	var ws strings.Builder
	ws.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetPr><pageSetUpPr fitToPage="1"/></sheetPr><sheetViews><sheetView workbookViewId="0" showGridLines="` + showGrid + `">`)
	if headerRow >= 0 {
		fmt.Fprintf(&ws, `<pane ySplit="%d" topLeftCell="A%d" activePane="bottomLeft" state="frozen"/><selection pane="bottomLeft" activeCell="A%d" sqref="A%d"/>`, headerRow+1, headerRow+2, headerRow+2, headerRow+2)
	}
	ws.WriteString(`</sheetView></sheetViews><cols>`)
	for i, w := range cols {
		fmt.Fprintf(&ws, `<col min="%d" max="%d" width="%g" customWidth="1"/>`, i+1, i+1, w)
	}
	ws.WriteString(`</cols><sheetData>`)
	for r, row := range rows {
		switch {
		case len(row) > 0 && row[0].style == xsTitle:
			fmt.Fprintf(&ws, `<row r="%d" ht="26" customHeight="1">`, r+1)
		case len(row) > 0 && (row[0].style == xsHeader || row[0].style == xsHeaderRight):
			fmt.Fprintf(&ws, `<row r="%d" ht="22" customHeight="1">`, r+1)
		default:
			fmt.Fprintf(&ws, `<row r="%d">`, r+1)
		}
		for c, cell := range row {
			ref := fmt.Sprintf("%c%d", 'A'+c, r+1)
			switch {
			case cell.num != "":
				fmt.Fprintf(&ws, `<c r="%s" s="%d"><v>%s</v></c>`, ref, cell.style+1, cell.num)
			case cell.text != "":
				fmt.Fprintf(&ws, `<c r="%s" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, cell.style+1, xmlText(cell.text))
			default: // vazia, mas com estilo: a borda da tabela não falha
				fmt.Fprintf(&ws, `<c r="%s" s="%d"/>`, ref, cell.style+1)
			}
		}
		ws.WriteString(`</row>`)
	}
	ws.WriteString(`</sheetData>`)
	quoted := xmlText("'" + strings.ReplaceAll(sheet, "'", "''") + "'")
	var names string
	if headerRow >= 0 {
		names = fmt.Sprintf(`<definedName name="_xlnm.Print_Titles" localSheetId="0">%s!$%d:$%d</definedName>`, quoted, headerRow+1, headerRow+1)
		if len(rows)-headerRow > 2 {
			filter := fmt.Sprintf("A%d:%s%d", headerRow+1, lastCol, len(rows))
			fmt.Fprintf(&ws, `<autoFilter ref="%s"/>`, filter)
			names = fmt.Sprintf(`<definedName name="_xlnm._FilterDatabase" localSheetId="0" hidden="1">%s!$A$%d:$%s$%d</definedName>`, quoted, headerRow+1, lastCol, len(rows)) + names
		}
		names = "<definedNames>" + names + "</definedNames>"
	}
	if len(merges) > 0 {
		fmt.Fprintf(&ws, `<mergeCells count="%d">`, len(merges))
		for _, m := range merges {
			fmt.Fprintf(&ws, `<mergeCell ref="%s"/>`, m)
		}
		ws.WriteString(`</mergeCells>`)
	}
	fmt.Fprintf(&ws, `<pageMargins left="0.4" right="0.4" top="0.6" bottom="0.6" header="0.3" footer="0.3"/><pageSetup paperSize="9" orientation="%s" fitToWidth="1" fitToHeight="0"/><headerFooter><oddFooter>&amp;L&amp;8%s&amp;R&amp;8Página &amp;P de &amp;N</oddFooter></headerFooter></worksheet>`, orientation, xmlText(strings.ReplaceAll(sheet, "&", "&&")))

	files := []struct{ name, body string }{
		{"[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`},
		{"_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="` + xmlText(sheet) + `" sheetId="1" r:id="rId1"/></sheets>` + names + `</workbook>`},
		{"xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", xlsxStyles},
		{"xl/worksheets/sheet1.xml", ws.String()},
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := z.Create(f.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(f.body)); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
