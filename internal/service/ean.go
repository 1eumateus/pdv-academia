// Caminho: pdv/internal/service/ean.go

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"pdv/internal/models"
	"pdv/internal/repository"
)

// EAN13Check calcula o dígito verificador de 12 dígitos (pesos 1 e 3).
func EAN13Check(d12 string) byte {
	sum := 0
	for i, c := range d12 {
		n := int(c - '0')
		if i%2 == 1 {
			n *= 3
		}
		sum += n
	}
	return byte('0' + (10-sum%10)%10)
}

// ValidEAN13 confere tamanho, dígitos e verificador.
func ValidEAN13(s string) bool {
	if len(s) != 13 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return EAN13Check(s[:12]) == s[12]
}

// internalEAN: prefixo 2 (uso interno da loja, nunca colide com produto de
// fábrica) + 11 dígitos + verificador.
func internalEAN(seq int64) string {
	d12 := fmt.Sprintf("2%011d", seq%100_000_000_000)
	return d12 + string(EAN13Check(d12))
}

// settingCodeSeq guarda a próxima sequência de código interno a tentar;
// a primeira é firstCodeSeq (SKU 2200, 2201, ...).
const (
	settingCodeSeq = "codigo_interno_seq"
	firstCodeSeq   = 2200
)

// seqCode: a sequência vira o SKU (só números) e o EAN interno do mesmo número.
func seqCode(seq int64) models.ProductCode {
	return models.ProductCode{Sku: strconv.FormatInt(seq, 10), Barcode: internalEAN(seq)}
}

// nextCodes devolve n sequências cujo SKU e EAN não pertencem a nenhum produto
// (a busca por código olha código de barras e SKU juntos, então confere os dois
// contra as duas colunas). Com reserve, o contador passa da última.
func (s *ProductService) nextCodes(ctx context.Context, q repository.DBTX, n int, reserve bool) ([]int64, error) {
	start := int64(firstCodeSeq)
	if v, ok, err := s.settings.Get(ctx, q, settingCodeSeq); err != nil {
		return nil, err
	} else if x, err := strconv.ParseInt(v, 10, 64); ok && err == nil && x > start {
		start = x
	}
	all, err := s.products.List(ctx, q)
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, p := range all {
		if p.Barcode != nil {
			used[*p.Barcode] = true
		}
		if p.Sku != nil {
			used[*p.Sku] = true
		}
	}
	out := make([]int64, 0, n)
	for seq := start; len(out) < n; seq++ {
		if seq >= 100_000_000_000 {
			return nil, newErr(CodeStateConflict, "Não há mais códigos internos livres.")
		}
		if c := seqCode(seq); !used[c.Sku] && !used[c.Barcode] {
			out = append(out, seq)
		}
	}
	if reserve && n > 0 {
		if err := s.settings.Set(ctx, q, settingCodeSeq, strconv.FormatInt(out[n-1]+1, 10)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SuggestCode mostra o próximo código livre no formulário, sem reservá-lo.
func (s *ProductService) SuggestCode(ctx context.Context) (models.ProductCode, error) {
	seqs, err := s.nextCodes(ctx, s.DB, 1, false)
	if err != nil {
		return models.ProductCode{}, err
	}
	return seqCode(seqs[0]), nil
}

// ReserveCodes separa n códigos para a planilha modelo: ninguém mais os recebe,
// mesmo que a planilha demore a voltar.
func (s *ProductService) ReserveCodes(ctx context.Context, n int) ([]models.ProductCode, error) {
	var out []models.ProductCode
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if _, err := requirePerm(ctx, s.Now(), PermProductCreate); err != nil {
			return err
		}
		seqs, err := s.nextCodes(ctx, tx, n, true)
		for _, seq := range seqs {
			out = append(out, seqCode(seq))
		}
		return err
	})
	return out, err
}

// AssignInternalBarcodes dá um EAN-13 interno a cada produto escolhido que
// ainda não tem código de barras. Devolve os produtos atualizados.
func (s *ProductService) AssignInternalBarcodes(ctx context.Context, ids []int64) ([]models.Product, error) {
	now := s.Now()
	authBy, err := requirePerm(ctx, now, PermProductCreate)
	if err != nil {
		return nil, err
	}
	var out []models.Product
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		for _, id := range ids {
			p, err := s.products.GetByID(ctx, tx, id)
			if err != nil {
				return fromRepo(err, notFoundErr(CodeProductNotFound, "Produto", id), "")
			}
			if p.Barcode == nil {
				seqs, err := s.nextCodes(ctx, tx, 1, true)
				if err != nil {
					return err
				}
				code := internalEAN(seqs[0])
				p.Barcode = &code
				f := repository.ProductFields{Name: p.Name, Barcode: p.Barcode, Sku: p.Sku, PriceCents: p.PriceCents, Active: p.Active,
					CategoryID: p.CategoryID, CostCents: p.CostCents, MinStock: p.MinStock, PricePending: p.PricePending}
				if err := s.products.Update(ctx, tx, id, f, now); err != nil {
					return fromRepo(err, nil, duplicateBarcodeMsg(f))
				}
				if err := audit(ctx, tx, now, authBy, "codigo_interno_gerado", "produto", &id, nil, map[string]any{"codigo": *p.Barcode}); err != nil {
					return err
				}
			}
			out = append(out, p)
		}
		return nil
	})
	return hideCosts(ctx, out), err
}
