// Caminho: pdv/internal/printer/escpos.go

// Package printer converte comprovantes em comandos ESC/POS e os envia em modo
// RAW para uma impressora térmica instalada no Windows.
package printer

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"pdv/internal/models"
)

type Options struct {
	Width      string // "58" ou "80"
	Cut        bool
	OpenDrawer bool
}

// columns e dots da fonte A padrão: 58 mm = 32 colunas / 384 pontos; 80 mm = 48 / 576.
func (o Options) columns() int {
	if o.Width == "58" {
		return 32
	}
	return 48
}

func (o Options) dots() int {
	if o.Width == "58" {
		return 384
	}
	return 576
}

const (
	escInit       = "\x1b@"
	escAlignLeft  = "\x1ba\x00"
	escAlignMid   = "\x1ba\x01"
	escBoldOn     = "\x1bE\x01"
	escBoldOff    = "\x1bE\x00"
	escFeed4      = "\x1bd\x04"
	gsCutFeed     = "\x1dVB\x00"        // avança até a guilhotina e corta (corte parcial)
	escDrawerKick = "\x1bp\x00\x19\xfa" // pulso no pino 2 da gaveta
)

// shortcut: acentos viram letras sem acento porque cada fabricante (Epson,
// Elgin, Bematech) numera as páginas de código de um jeito; se algum cliente
// exigir acentos, configurar a página (ESC t n) por modelo de impressora.
var asciiFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a", "Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"é", "e", "ê", "e", "è", "e", "É", "E", "Ê", "E", "È", "E",
	"í", "i", "î", "i", "Í", "I", "Î", "I",
	"ó", "o", "ô", "o", "õ", "o", "ö", "o", "Ó", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"ú", "u", "ü", "u", "Ú", "U", "Ü", "U", "ç", "c", "Ç", "C", "ñ", "n", "Ñ", "N",
	"º", "o", "ª", "a", "—", "-", "–", "-", "·", "-", "“", "\"", "”", "\"", "‘", "'", "’", "'",
)

func toASCII(s string) string {
	s = asciiFold.Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteByte(' ')
		case r >= 32 && r < 127:
			b.WriteRune(r)
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}

// wrap quebra o texto em linhas de até width colunas, por palavra.
func wrap(s string, width int) []string {
	var out []string
	line := ""
	for _, w := range strings.Fields(s) {
		for len(w) > width { // palavra maior que a linha: quebra no meio
			if line != "" {
				out, line = append(out, line), ""
			}
			out, w = append(out, w[:width]), w[width:]
		}
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= width:
			line += " " + w
		default:
			out, line = append(out, line), w
		}
	}
	if line != "" || len(out) == 0 {
		out = append(out, line)
	}
	return out
}

// itemRow monta produto, qtd, preço e total numa linha de cols caracteres; o nome
// é cortado para caber (58 mm: 11 letras; 80 mm: 22).
func itemRow(l models.ReceiptLine, cols int) string {
	wq, wp, wt := 3, 7, 8
	if cols >= 48 {
		wq, wp, wt = 4, 9, 10
	}
	wn := cols - wq - wp - wt - 3
	name := toASCII(l.Text)
	if len(name) > wn {
		name = name[:wn]
	}
	return fmt.Sprintf("%-*s %*s %*s %*s", wn, name, wq, toASCII(l.Qty), wp, toASCII(l.Price), wt, toASCII(l.Right))
}

// Render gera os bytes ESC/POS do comprovante.
func Render(r models.Receipt, o Options) []byte {
	cols := o.columns()
	var b bytes.Buffer
	b.WriteString(escInit)
	if img := decodeLogo(r.Logo); img != nil {
		b.WriteString(escAlignMid)
		b.Write(raster(img, o.dots()))
		b.WriteString("\n")
	}
	for _, l := range r.Lines {
		if l.Bold {
			b.WriteString(escBoldOn)
		}
		switch l.Kind {
		case models.LineSeparator:
			b.WriteString(escAlignLeft + strings.Repeat("-", cols) + "\n")
		case models.LinePair:
			b.WriteString(escAlignLeft)
			left, right := toASCII(l.Text), toASCII(l.Right)
			if len(left)+1+len(right) <= cols {
				b.WriteString(left + strings.Repeat(" ", cols-len(left)-len(right)) + right + "\n")
			} else {
				for _, w := range wrap(left, cols) {
					b.WriteString(w + "\n")
				}
				b.WriteString(strings.Repeat(" ", max(0, cols-len(right))) + right + "\n")
			}
		case models.LineItem:
			b.WriteString(escAlignLeft + itemRow(l, cols) + "\n")
		case models.LineQR:
			b.WriteString(escAlignMid)
			b.Write(qrCommand(l.Text))
			b.WriteString("\n")
		case models.LineSignature:
			b.WriteString(escAlignMid + "\n\n" + strings.Repeat("_", cols-6) + "\n" + toASCII(l.Text) + "\n")
		default:
			if l.Center {
				b.WriteString(escAlignMid)
			} else {
				b.WriteString(escAlignLeft)
			}
			for _, w := range wrap(toASCII(l.Text), cols) {
				b.WriteString(w + "\n")
			}
		}
		if l.Bold {
			b.WriteString(escBoldOff)
		}
	}
	b.WriteString(escAlignLeft)
	if o.OpenDrawer {
		b.WriteString(escDrawerKick)
	}
	if o.Cut {
		b.WriteString(escFeed4 + gsCutFeed)
	} else {
		b.WriteString(escFeed4)
	}
	return b.Bytes()
}

// decodeLogo lê uma data URL PNG/JPEG. Logo inválida é ignorada: o comprovante
// sai sem ela em vez de falhar.
func decodeLogo(dataURL string) image.Image {
	i := strings.Index(dataURL, ";base64,")
	if i < 0 {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[i+len(";base64,"):])
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	return img
}

// raster reduz a imagem (no máximo 60% da largura do papel e 160 pontos de
// altura), converte para preto e branco e gera o comando GS v 0.
func raster(img image.Image, paperDots int) []byte {
	src := img.Bounds()
	if src.Dx() == 0 || src.Dy() == 0 {
		return nil
	}
	maxW, maxH := paperDots*6/10, 160
	w, h := src.Dx(), src.Dy()
	if w > maxW {
		w, h = maxW, h*maxW/src.Dx()
	}
	if h > maxH {
		w, h = w*maxH/h, maxH
	}
	w, h = max(w, 1), max(h, 1)
	rowBytes := (w + 7) / 8
	out := []byte{0x1d, 'v', '0', 0, byte(rowBytes), byte(rowBytes >> 8), byte(h), byte(h >> 8)}
	for y := 0; y < h; y++ {
		row := make([]byte, rowBytes)
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(src.Min.X+x*src.Dx()/w, src.Min.Y+y*src.Dy()/h).RGBA()
			lum := (299*r + 587*g + 114*b) / 1000 // 0..65535
			if a > 0x8000 && lum < 0x8000 {
				row[x/8] |= 0x80 >> (x % 8)
			}
		}
		out = append(out, row...)
	}
	return out
}

// qrCommand usa o QR nativo da impressora (GS ( k, padrão Epson, aceito por
// Elgin e Bematech): modelo 2, módulo 6, correção M.
func qrCommand(data string) []byte {
	n := len(data) + 3
	out := []byte{
		0x1d, '(', 'k', 4, 0, 49, 65, 50, 0, // modelo 2
		0x1d, '(', 'k', 3, 0, 49, 67, 6, // tamanho do módulo
		0x1d, '(', 'k', 3, 0, 49, 69, 49, // correção de erro M
		0x1d, '(', 'k', byte(n), byte(n >> 8), 49, 80, 48,
	}
	out = append(out, data...)
	return append(out, 0x1d, '(', 'k', 3, 0, 49, 81, 48) // imprime
}
