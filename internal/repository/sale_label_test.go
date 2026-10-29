package repository

import "testing"

func TestItemsLabel(t *testing.T) {
	cases := []struct {
		name       string
		qty, count int64
		want       string
	}{
		{"Coca-Cola 2L", 1, 1, "Coca-Cola 2L"},
		{"Coca-Cola 2L", 3, 1, "3x Coca-Cola 2L"},
		{"Coca-Cola 2L", 3, 2, "3x Coca-Cola 2L + 1 item"},
		{"Coca-Cola 2L", 1, 3, "Coca-Cola 2L + 2 itens"},
		{"", 0, 0, ""},
	}
	for _, c := range cases {
		if got := itemsLabel(c.name, c.qty, c.count); got != c.want {
			t.Errorf("itemsLabel(%q, %d, %d) = %q, want %q", c.name, c.qty, c.count, got, c.want)
		}
	}
}
