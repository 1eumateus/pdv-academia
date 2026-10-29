// Caminho: pdv/internal/repository/saved_report_repo.go

package repository

import (
	"context"
	"database/sql"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type SavedReportRepo struct{}

const savedReportSelect = `SELECT r.id, r.kind, r.start_date, r.end_date, f.name, r.sale_count, r.total_cents, r.user_id, u.name, r.created_at
	FROM saved_reports r LEFT JOIN users f ON f.id = r.filter_user_id LEFT JOIN users u ON u.id = r.user_id`

func scanSavedReport(s interface{ Scan(...any) error }) (models.SavedReport, error) {
	var (
		v            models.SavedReport
		uid          sql.NullInt64
		fname, uname sql.NullString
		created      string
	)
	if err := s.Scan(&v.ID, &v.Kind, &v.StartDate, &v.EndDate, &fname, &v.SaleCount, &v.TotalCents, &uid, &uname, &created); err != nil {
		return v, err
	}
	v.FilterUserName, v.UserID, v.UserName = nullStr(fname), nullInt(uid), nullStr(uname)
	var err error
	v.CreatedAt, err = parseTime(created)
	return v, err
}

// Insert grava a cópia do relatório. filterUserID nil = todos os operadores.
func (SavedReportRepo) Insert(ctx context.Context, q DBTX, kind string, r models.SalesReport, filterUserID *int64, data string, now time.Time) (int64, error) {
	const op = "salvar relatório"
	res, err := q.ExecContext(ctx,
		`INSERT INTO saved_reports (kind, start_date, end_date, filter_user_id, sale_count, total_cents, data, user_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		kind, r.StartDate, r.EndDate, filterUserID, r.SaleCount, r.TotalCents, data, actor.ID(ctx), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

// List devolve os mais recentes primeiro. ownerID 0 = todos; senão só os gerados por ele.
func (SavedReportRepo) List(ctx context.Context, q DBTX, ownerID int64, limit int) ([]models.SavedReport, error) {
	const op = "listar relatórios gerados"
	rows, err := q.QueryContext(ctx, savedReportSelect+` WHERE (? = 0 OR r.user_id = ?) ORDER BY r.id DESC LIMIT ?`, ownerID, ownerID, limit)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.SavedReport{}
	for rows.Next() {
		v, err := scanSavedReport(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, v)
	}
	return out, wrap(op, rows.Err())
}

// Get devolve a linha e a cópia em JSON.
func (SavedReportRepo) Get(ctx context.Context, q DBTX, id int64) (models.SavedReport, string, error) {
	const op = "ler relatório gerado"
	var data string
	v, err := scanSavedReport(q.QueryRowContext(ctx, savedReportSelect+` WHERE r.id = ?`, id))
	if err != nil {
		return v, "", wrap(op, err)
	}
	err = q.QueryRowContext(ctx, `SELECT data FROM saved_reports WHERE id = ?`, id).Scan(&data)
	return v, data, wrap(op, err)
}
