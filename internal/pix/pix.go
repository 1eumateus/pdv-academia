// Caminho: pdv/internal/pix/pix.go

// Package pix gera o "Pix copia e cola" estático (BR Code, padrão EMV do Banco
// Central) com valor, e a imagem do QR Code. Não fala com banco: o operador
// confere o recebimento no app do banco.
package pix

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const (
	maxName  = 25
	maxCity  = 15
	maxTxID  = 25
	maxValue = 99_999_999_999 // R$ 999.999.999,99 (13 caracteres no campo 54)
)

// field monta um campo EMV: ID (2) + tamanho (2) + valor.
func field(id, value string) string { return fmt.Sprintf("%s%02d%s", id, len(value), value) }

// CRC16 é o CRC16-CCITT-FALSE (polinômio 0x1021, inicial 0xFFFF) exigido no campo 63.
func CRC16(s string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for b := 0; b < 8; b++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// ASCII remove acentos e deixa só caracteres que os apps de banco aceitam.
func ASCII(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	out, _, _ := transform.String(t, s)
	var b strings.Builder
	for _, r := range out {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

var txidChars = regexp.MustCompile(`[^A-Za-z0-9]`)

// Payload devolve o "copia e cola". amountCents 0 = sem valor fixo; txid vazio = "***".
func Payload(key, name, city string, amountCents int64, txid string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("chave Pix não configurada")
	}
	name, city = ASCII(name), ASCII(city)
	if name == "" || city == "" {
		return "", errors.New("informe o nome do recebedor e a cidade do Pix")
	}
	if len(name) > maxName {
		name = name[:maxName]
	}
	if len(city) > maxCity {
		city = city[:maxCity]
	}
	if amountCents < 0 || amountCents > maxValue {
		return "", errors.New("valor do Pix inválido")
	}
	txid = txidChars.ReplaceAllString(txid, "")
	if len(txid) > maxTxID {
		txid = txid[:maxTxID]
	}
	if txid == "" {
		txid = "***"
	}
	var b strings.Builder
	b.WriteString(field("00", "01"))
	b.WriteString(field("26", field("00", "br.gov.bcb.pix")+field("01", key)))
	b.WriteString(field("52", "0000"))
	b.WriteString(field("53", "986"))
	if amountCents > 0 {
		b.WriteString(field("54", fmt.Sprintf("%d.%02d", amountCents/100, amountCents%100)))
	}
	b.WriteString(field("58", "BR"))
	b.WriteString(field("59", name))
	b.WriteString(field("60", city))
	b.WriteString(field("62", field("05", txid)))
	b.WriteString("6304")
	s := b.String()
	return s + fmt.Sprintf("%04X", CRC16(s)), nil
}

// QRDataURL gera a imagem PNG do QR em data URL para a tela.
func QRDataURL(payload string, size int) (string, error) {
	png, err := qrcode.Encode(payload, qrcode.Medium, size)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

var (
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reEVP   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

// NormalizeKey confere o formato da chave e devolve como deve ir no código:
// CPF/CNPJ só dígitos, telefone com +55, e-mail em minúsculas, aleatória como está.
// validDoc confere os dígitos verificadores de CPF (11) e CNPJ (14).
func NormalizeKey(key string, validDoc func(string) bool) (string, error) {
	k := strings.TrimSpace(key)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, k)
	phone := func() (string, error) {
		d := digits
		if len(d) == 12 || len(d) == 13 {
			d = strings.TrimPrefix(d, "55")
		}
		if len(d) != 10 && len(d) != 11 {
			return "", errors.New("telefone da chave Pix inválido: use DDD + número")
		}
		return "+55" + d, nil
	}
	doc := func() (string, error) {
		if (len(digits) != 11 && len(digits) != 14) || !validDoc(digits) {
			return "", errors.New("CPF/CNPJ da chave Pix inválido")
		}
		return digits, nil
	}
	switch {
	case reEVP.MatchString(k):
		return strings.ToLower(k), nil
	case reEmail.MatchString(k):
		if len(k) > 77 {
			return "", errors.New("e-mail da chave Pix grande demais")
		}
		return strings.ToLower(k), nil
	case strings.ContainsAny(k, "./"): // formato de CPF/CNPJ
		return doc()
	case strings.HasPrefix(k, "+") || strings.ContainsAny(k, "() "): // formato de telefone
		return phone()
	case digits == k && (len(k) == 11 || len(k) == 14) && validDoc(k):
		return k, nil
	case digits == k && len(k) >= 10 && len(k) <= 13:
		return phone()
	}
	return "", errors.New("chave Pix inválida: use CPF, CNPJ, e-mail, telefone (+55 DDD número) ou chave aleatória")
}
