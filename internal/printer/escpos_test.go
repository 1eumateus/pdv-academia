// Caminho: pdv/internal/printer/escpos_test.go

package printer

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"pdv/internal/models"
)

func TestRenderLayout(t *testing.T) {
	r := models.Receipt{Lines: []models.ReceiptLine{
		{Kind: models.LineText, Text: "Pão de açúcar", Center: true, Bold: true},
		{Kind: models.LinePair, Text: "TOTAL", Right: "R$ 10,00"},
		{Kind: models.LinePair, Text: strings.Repeat("palavra ", 10), Right: "R$ 1,00"},
		{Kind: models.LineSeparator},
	}}
	out := string(Render(r, Options{Width: "58"}))
	if !strings.Contains(out, "\x1bE\x01\x1ba\x01Pao de acucar\n\x1bE\x00") {
		t.Errorf("texto centralizado/negrito ou acentos errados: %q", out)
	}
	if !strings.Contains(out, "TOTAL"+strings.Repeat(" ", 32-5-8)+"R$ 10,00\n") {
		t.Errorf("par não alinhado em 32 colunas: %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		plain := strings.NewReplacer("\x1ba\x00", "", "\x1ba\x01", "", "\x1bE\x01", "", "\x1bE\x00", "", "\x1b@", "").Replace(line)
		if len(plain) > 32 && !strings.ContainsAny(plain, "\x1b\x1d") {
			t.Errorf("linha passou de 32 colunas: %q", plain)
		}
	}
	if !strings.Contains(out, strings.Repeat("-", 32)+"\n") {
		t.Error("separador deveria ter 32 colunas")
	}
	if strings.Contains(out, "\x1dV") || strings.Contains(out, "\x1bp") {
		t.Error("não deveria cortar nem abrir gaveta sem opção")
	}
	if full := string(Render(r, Options{Width: "80", Cut: true, OpenDrawer: true})); !strings.Contains(full, escDrawerKick) ||
		!strings.HasSuffix(full, gsCutFeed) {
		t.Error("deveria abrir a gaveta e cortar no fim")
	}
}

func TestWrap(t *testing.T) {
	got := wrap("abc defghijkl mn", 5)
	want := []string{"abc", "defgh", "ijkl", "mn"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("wrap = %v, queria %v", got, want)
	}
}

func TestLogoRaster(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1000, 100))
	for x := 0; x < 1000; x++ {
		for y := 0; y < 100; y++ {
			c := color.RGBA{255, 255, 255, 255}
			if x < 500 {
				c = color.RGBA{0, 0, 0, 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	logo := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	out := Render(models.Receipt{Logo: logo}, Options{Width: "80"})
	i := bytes.Index(out, []byte{0x1d, 'v', '0', 0})
	if i < 0 {
		t.Fatal("logo não foi enviada como imagem")
	}
	w := int(out[i+4]) | int(out[i+5])<<8
	h := int(out[i+6]) | int(out[i+7])<<8
	// 1000 px reduz para 60% de 576 = 345 pontos (44 bytes por linha), mantendo a proporção.
	if w != 44 || h != 34 {
		t.Fatalf("tamanho da logo = %d bytes x %d linhas", w, h)
	}
	row := out[i+8 : i+8+w]
	if row[0] != 0xff || row[w-1] != 0 {
		t.Fatalf("metade esquerda deveria ser preta e a direita branca: %x", row)
	}

	if bytes.Contains(Render(models.Receipt{Logo: "data:image/png;base64,lixo"}, Options{}), []byte{0x1d, 'v', '0'}) {
		t.Fatal("logo inválida deveria ser ignorada")
	}
}

func TestItemRowOneLine(t *testing.T) {
	l := models.ReceiptLine{Kind: models.LineItem, Text: "Café torrado e moído 500g extra forte", Qty: "2", Price: "15,00", Right: "30,00"}
	for _, cols := range []int{32, 48} {
		row := itemRow(l, cols)
		if len(row) != cols || !strings.HasSuffix(row, " 2   15,00    30,00") && !strings.HasSuffix(row, "    2     15,00      30,00") {
			t.Fatalf("%d colunas: %q", cols, row)
		}
		if !strings.HasPrefix(row, "Cafe torrad") {
			t.Fatalf("nome: %q", row)
		}
	}
}
