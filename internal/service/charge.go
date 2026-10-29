// Caminho: pdv/internal/service/charge.go

package service

import (
	"context"
	"net/url"
	"strings"
)

// whatsappPhone devolve o número no formato do wa.me (55 + DDD + número), ou
// "" se o telefone não tiver DDD + 8 ou 9 dígitos.
func whatsappPhone(phone string) string {
	d := onlyDigits(phone)
	if len(d) >= 12 && strings.HasPrefix(d, "55") {
		d = d[2:]
	}
	d = strings.TrimPrefix(d, "0") // 0 + DDD de discagem interurbana
	if len(d) != 10 && len(d) != 11 {
		return ""
	}
	return "55" + d
}

// ChargeLink monta o link do WhatsApp com a mensagem de cobrança do a prazo
// (Configurações) e registra a data da cobrança. Quem envia é o operador.
func (s *CustomerService) ChargeLink(ctx context.Context, id int64) (string, error) {
	c, err := s.customers.GetByID(ctx, s.DB, id)
	if err != nil {
		return "", fromRepo(err, notFoundErr(CodeCustomerNotFound, "Cliente", id), "")
	}
	if c.CreditBalanceCents <= 0 {
		return "", invalid("%s não tem saldo em aberto a prazo.", c.Name)
	}
	phone := ""
	if c.Phone != nil {
		phone = whatsappPhone(*c.Phone)
	}
	if phone == "" {
		return "", invalid("Cadastre o telefone de %s com DDD (ex.: 11 98765-4321) para cobrar pelo WhatsApp.", c.Name)
	}
	st, err := (&SettingsService{Base: s.Base}).Get(ctx)
	if err != nil {
		return "", err
	}
	msg := strings.NewReplacer("{nome}", strings.Fields(c.Name)[0], "{saldo}", brl(c.CreditBalanceCents), "{loja}", st.StoreName).
		Replace(st.ChargeMessage)
	if err := s.customers.SetLastCharge(ctx, s.DB, id, s.Now()); err != nil {
		return "", err
	}
	return "https://wa.me/" + phone + "?text=" + strings.ReplaceAll(url.QueryEscape(msg), "+", "%20"), nil
}
