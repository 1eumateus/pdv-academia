// Caminho: pdv/internal/pix/pix_test.go

package pix

import (
	"strings"
	"testing"
)

func TestCRC16(t *testing.T) {
	// Valor de verificação padrão do CRC-16/CCITT-FALSE.
	if got := CRC16("123456789"); got != 0x29B1 {
		t.Fatalf("CRC16 = %04X, esperado 29B1", got)
	}
}

func TestPayloadBCBExample(t *testing.T) {
	// Exemplo do Manual de Padrões para Iniciação do Pix (BR Code estático, sem valor).
	got, err := Payload("123e4567-e12b-12d1-a456-426655440000", "Fulano de Tal", "BRASILIA", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "00020126580014br.gov.bcb.pix0136123e4567-e12b-12d1-a456-4266554400005204000053039865802BR5913Fulano de Tal6008BRASILIA62070503***63041D3D"
	if got != want {
		t.Fatalf("payload\n got %s\nwant %s", got, want)
	}
}

func TestPayloadWithAmountAndLimits(t *testing.T) {
	got, err := Payload("+5511999998888", "José da Conceição Comércio de Roupas", "São José dos Campos", 12345, "VENDA-0042/abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"5406123.45",                   // valor
		"5925Jose da Conceicao Comerc", // nome sem acento, cortado em 25
		"6015Sao Jose dos Ca",          // cidade cortada em 15
		"62160512VENDA0042abc",         // txid só com letras e números
	} {
		if !strings.Contains(got, want) {
			t.Errorf("payload sem %q:\n%s", want, got)
		}
	}
	body, crc := got[:len(got)-4], got[len(got)-4:]
	if !strings.HasSuffix(body, "6304") || crc != strings.ToUpper(crc) || len(crc) != 4 {
		t.Fatalf("CRC mal formado: %s", got)
	}
	if _, err := Payload("", "Loja", "Cidade", 100, ""); err == nil {
		t.Fatal("sem chave deveria falhar")
	}
	if img, err := QRDataURL(got, 256); err != nil || !strings.HasPrefix(img, "data:image/png;base64,") {
		t.Fatalf("imagem do QR: %v", err)
	}
}

func TestNormalizeKey(t *testing.T) {
	okDoc := func(d string) bool { return d != "11111111111" }
	cases := map[string]string{
		"529.982.247-25":                       "52998224725",
		"Loja@Exemplo.com.br":                  "loja@exemplo.com.br",
		"(11) 99999-8888":                      "+5511999998888",
		"+55 11 99999-8888":                    "+5511999998888",
		"123E4567-E12B-12D1-A456-426655440000": "123e4567-e12b-12d1-a456-426655440000",
	}
	for in, want := range cases {
		if got, err := NormalizeKey(in, okDoc); err != nil || got != want {
			t.Errorf("NormalizeKey(%q) = %q, %v; queria %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"111.111.111-11", "abc", "+55 11 123"} {
		if _, err := NormalizeKey(bad, okDoc); err == nil {
			t.Errorf("NormalizeKey(%q) deveria falhar", bad)
		}
	}
}
