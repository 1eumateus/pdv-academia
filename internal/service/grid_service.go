// Caminho: pdv/internal/service/grid_service.go

package service

import (
	"context"
	"database/sql"
	"strings"

	"pdv/internal/models"
	"pdv/internal/repository"
)

const (
	maxGridValues     = 20 // tamanhos ou cores por grade
	maxGridValueLen   = 30
	maxGridVariations = 200
)

// cleanGridValues apara, remove repetidos (sem diferenciar maiúsculas) e limita.
func cleanGridValues(xs []string, label string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x == "" || seen[strings.ToLower(x)] {
			continue
		}
		if len([]rune(x)) > maxGridValueLen {
			return nil, invalid("Cada %s deve ter no máximo %d caracteres.", label, maxGridValueLen)
		}
		seen[strings.ToLower(x)] = true
		out = append(out, x)
	}
	if len(out) > maxGridValues {
		return nil, invalid("Use no máximo %d valores de %s.", maxGridValues, label)
	}
	return out, nil
}

// union devolve a com os valores de b que ainda não estão nela.
func union(a, b []string) []string {
	out := append([]string{}, a...)
	for _, x := range b {
		found := false
		for _, y := range out {
			found = found || strings.EqualFold(x, y)
		}
		if !found {
			out = append(out, x)
		}
	}
	return out
}

func variationName(model, size, color string) string {
	parts := []string{model}
	for _, p := range []string{size, color} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// SaveGrid cria uma grade (ModelID 0) gerando uma variação por tamanho × cor, ou
// acrescenta tamanhos/cores a uma grade existente criando só as combinações que faltam.
func (s *ProductService) SaveGrid(ctx context.Context, in models.GridInput) (models.ProductModel, error) {
	sizes, err := cleanGridValues(in.Sizes, "tamanho")
	if err != nil {
		return models.ProductModel{}, err
	}
	colors, err := cleanGridValues(in.Colors, "cor")
	if err != nil {
		return models.ProductModel{}, err
	}
	var id int64
	err = s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		if _, err := requirePerm(ctx, now, PermProductCreate); err != nil {
			return err
		}
		var m models.ProductModel
		existing := map[[2]string]bool{}
		action := "grade_ampliada"
		if in.ModelID == 0 {
			if len(sizes) == 0 && len(colors) == 0 {
				return invalid("Informe ao menos um tamanho ou uma cor.")
			}
			name, err := cleanName(in.Name, "nome do modelo")
			if err != nil {
				return err
			}
			if err := validatePrice(in.BasePriceCents); err != nil {
				return err
			}
			catID := validCustomerOrNil(in.CategoryID)
			if err := s.checkCategory(ctx, tx, catID, nil); err != nil {
				return err
			}
			if m.ID, err = s.models.Insert(ctx, tx, name, catID, in.BasePriceCents, now); err != nil {
				return fromRepo(err, nil, "Já existe um modelo com este nome.")
			}
			m.Name, m.CategoryID, m.BasePriceCents = name, catID, in.BasePriceCents
			action = "grade_criada"
		} else {
			if m, err = s.models.GetByID(ctx, tx, in.ModelID); err != nil {
				return fromRepo(err, newErr(CodeNotFound, "Modelo %d não encontrado.", in.ModelID), "")
			}
			vars, err := s.models.Variations(ctx, tx, m.ID)
			if err != nil {
				return err
			}
			var oldSizes, oldColors []string
			for _, v := range vars {
				existing[[2]string{strings.ToLower(v[0]), strings.ToLower(v[1])}] = true
				oldSizes, oldColors = union(oldSizes, nonEmpty(v[0])), union(oldColors, nonEmpty(v[1]))
			}
			sizes, colors = union(oldSizes, sizes), union(oldColors, colors)
		}
		id = m.ID

		if len(sizes) == 0 {
			sizes = []string{""}
		}
		if len(colors) == 0 {
			colors = []string{""}
		}
		if len(sizes)*len(colors) > maxGridVariations {
			return invalid("A grade passaria de %d variações. Divida em mais de um modelo.", maxGridVariations)
		}
		created := 0
		for _, sz := range sizes {
			for _, co := range colors {
				if existing[[2]string{strings.ToLower(sz), strings.ToLower(co)}] {
					continue
				}
				if _, err := s.products.Insert(ctx, tx, repository.ProductFields{
					Name: variationName(m.Name, sz, co), PriceCents: m.BasePriceCents, Active: true,
					CategoryID: m.CategoryID, ModelID: &m.ID, Size: sz, Color: co,
				}, now); err != nil {
					return err
				}
				created++
			}
		}
		if created == 0 {
			return invalid("Nenhuma variação nova: todas essas combinações já existem.")
		}
		return audit(ctx, tx, now, nil, action, "modelo", &m.ID, nil,
			map[string]any{"modelo": m.Name, "variacoes_criadas": created})
	})
	if err != nil {
		return models.ProductModel{}, err
	}
	return s.GetModel(ctx, id)
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

// GetModel devolve o modelo com os tamanhos e cores que já existem na grade.
func (s *ProductService) GetModel(ctx context.Context, id int64) (models.ProductModel, error) {
	m, err := s.models.GetByID(ctx, s.DB, id)
	if err != nil {
		return m, fromRepo(err, newErr(CodeNotFound, "Modelo %d não encontrado.", id), "")
	}
	vars, err := s.models.Variations(ctx, s.DB, id)
	if err != nil {
		return m, err
	}
	m.Sizes, m.Colors = []string{}, []string{}
	for _, v := range vars {
		m.Sizes, m.Colors = union(m.Sizes, nonEmpty(v[0])), union(m.Colors, nonEmpty(v[1]))
	}
	m.VariationCount = int64(len(vars))
	return m, nil
}

// ApplyGridPrice põe o mesmo preço em todas as variações e no preço base do modelo.
func (s *ProductService) ApplyGridPrice(ctx context.Context, modelID, priceCents int64) (models.ProductModel, error) {
	if err := validatePrice(priceCents); err != nil {
		return models.ProductModel{}, err
	}
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		now := s.Now()
		m, err := s.models.GetByID(ctx, tx, modelID)
		if err != nil {
			return fromRepo(err, newErr(CodeNotFound, "Modelo %d não encontrado.", modelID), "")
		}
		authBy, err := requirePerm(ctx, now, PermPrice)
		if err != nil {
			return err
		}
		if err := audit(ctx, tx, now, authBy, "preco_grade_alterado", "modelo", &modelID,
			map[string]any{"preco_base": m.BasePriceCents}, map[string]any{"preco": priceCents}); err != nil {
			return err
		}
		if err := s.models.SetPriceAll(ctx, tx, modelID, priceCents, now); err != nil {
			return err
		}
		return s.models.SetBasePrice(ctx, tx, modelID, priceCents, now)
	})
	if err != nil {
		return models.ProductModel{}, err
	}
	return s.GetModel(ctx, modelID)
}
