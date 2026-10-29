// Caminho: pdv/internal/service/charge_test.go

package service

import (
	"strings"
	"testing"

	"pdv/internal/models"
)

func TestWhatsappChargeLink(t *testing.T) {
	for in, want := range map[string]string{"(11) 98765-4321": "5511987654321", "+55 11 3333-4444": "551133334444",
		"011 98765-4321": "5511987654321", "98765-4321": ""} {
		if got := whatsappPhone(in); got != want {
			t.Fatalf("%s: %s, queria %s", in, got, want)
		}
	}

	e := newEnv(t).withCash()
	phone := "(11) 98765-4321"
	c, _ := e.customers.Create(e.ctx, models.CustomerInput{Name: "Maria Souza", Phone: &phone, CreditLimitCents: 100000, Active: true})
	if _, err := e.customers.ChargeLink(e.ctx, c.ID); err == nil {
		t.Fatal("cobrou cliente sem dívida")
	}
	p := e.product("Arroz", 2550, 10)
	e.sell("fiado", &c.ID, item(p.ID, 2))

	link, err := e.customers.ChargeLink(e.ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(link, "https://wa.me/5511987654321?text=Ol%C3%A1%2C%20Maria%21") || !strings.Contains(link, "R%24%2051%2C00") || strings.Contains(link, "+") {
		t.Fatalf("link: %s", link)
	}
	got, _ := e.customers.Get(e.ctx, c.ID)
	if got.LastChargeAt == nil {
		t.Fatal("não registrou a data da cobrança")
	}
}
