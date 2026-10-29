// Caminho: pdv/internal/repository/user_repo.go

package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"pdv/internal/actor"
	"pdv/internal/models"
)

type UserRepo struct{}

const userCols = `id, name, login, role, active, created_at, updated_at, permissions, COALESCE(session_id, '')`

func scanUser(s interface{ Scan(...any) error }) (models.User, error) {
	var (
		u                models.User
		active           int
		created, updated string
		perms            sql.NullString
		err              error
	)
	if err = s.Scan(&u.ID, &u.Name, &u.Login, &u.Role, &active, &created, &updated, &perms, &u.SessionID); err != nil {
		return u, err
	}
	u.Active = active == 1
	u.LoggedIn = u.SessionID != ""
	if perms.Valid {
		_ = json.Unmarshal([]byte(perms.String), &u.Permissions) // inválido = padrão do papel
	}
	if u.CreatedAt, err = parseTime(created); err != nil {
		return u, err
	}
	u.UpdatedAt, err = parseTime(updated)
	return u, err
}

func (UserRepo) Count(ctx context.Context, q DBTX) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, wrap("contar usuários", err)
}

// CountActiveAdmins conta administradores ativos, excluindo um id (0 = nenhum).
func (UserRepo) CountActiveAdmins(ctx context.Context, q DBTX, exceptID int64) (int64, error) {
	var n int64
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND active = 1 AND id <> ?`, exceptID).Scan(&n)
	return n, wrap("contar administradores", err)
}

// permsArg grava a lista em JSON; nil vira NULL (padrão do papel).
func permsArg(perms []string) any {
	if perms == nil {
		return nil
	}
	b, _ := json.Marshal(perms)
	return string(b)
}

func (UserRepo) Insert(ctx context.Context, q DBTX, name, login, hash, role string, active bool, perms []string, now time.Time) (int64, error) {
	const op = "cadastrar usuário"
	res, err := q.ExecContext(ctx,
		`INSERT INTO users (name, login, password_hash, role, active, permissions, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		name, login, hash, role, boolInt(active), permsArg(perms), FormatTime(now), FormatTime(now))
	if err != nil {
		return 0, wrap(op, err)
	}
	return lastID(op, res)
}

func (UserRepo) Update(ctx context.Context, q DBTX, id int64, name, login, role string, active bool, perms []string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET name = ?, login = ?, role = ?, active = ?, permissions = ?, updated_at = ? WHERE id = ?`,
		name, login, role, boolInt(active), permsArg(perms), FormatTime(now), id)
	return wrap("alterar usuário", err)
}

func (UserRepo) SetPassword(ctx context.Context, q DBTX, id int64, hash string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, FormatTime(now), id)
	return wrap("alterar senha", err)
}

func (UserRepo) PasswordHash(ctx context.Context, q DBTX, id int64) (string, error) {
	var h string
	err := q.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, id).Scan(&h)
	return h, wrap("buscar senha", err)
}

// SetSession marca o usuário como logado na instância sessionID.
func (UserRepo) SetSession(ctx context.Context, q DBTX, id int64, sessionID string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET session_id = ? WHERE id = ?`, sessionID, id)
	return wrap("registrar sessão", err)
}

// ClearSession tira a marca de logado; sessionID vazio libera qualquer sessão.
func (UserRepo) ClearSession(ctx context.Context, q DBTX, id int64, sessionID string) error {
	_, err := q.ExecContext(ctx, `UPDATE users SET session_id = NULL WHERE id = ? AND (? = '' OR session_id = ?)`, id, sessionID, sessionID)
	return wrap("encerrar sessão", err)
}

func (UserRepo) GetByID(ctx context.Context, q DBTX, id int64) (models.User, error) {
	u, err := scanUser(q.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
	return u, wrap("buscar usuário", err)
}

// GetByLogin devolve o usuário e o hash da senha (login sem diferenciar maiúsculas).
func (r UserRepo) GetByLogin(ctx context.Context, q DBTX, login string) (models.User, string, error) {
	var (
		id   int64
		hash string
	)
	if err := q.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE login = ?`, login).Scan(&id, &hash); err != nil {
		return models.User{}, "", wrap("buscar usuário", err)
	}
	u, err := r.GetByID(ctx, q, id)
	return u, hash, err
}

func (UserRepo) List(ctx context.Context, q DBTX) ([]models.User, error) {
	const op = "listar usuários"
	rows, err := q.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY active DESC, name COLLATE NOCASE`)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, u)
	}
	return out, wrap(op, rows.Err())
}

type AuditRepo struct{}

// Insert grava o registro com o usuário do context. before/after já vêm em JSON.
func (AuditRepo) Insert(ctx context.Context, q DBTX, authorizedBy *int64, action, entity string, entityID *int64,
	before, after *string, now time.Time) error {
	var ent any
	if entity != "" {
		ent = entity
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO audit_log (user_id, authorized_by_id, action, entity, entity_id, before_json, after_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		actor.ID(ctx), authorizedBy, action, ent, entityID, before, after, FormatTime(now))
	return wrap("registrar auditoria", err)
}

// List devolve a auditoria de [start, end), mais recente primeiro.
func (AuditRepo) List(ctx context.Context, q DBTX, start, end time.Time, limit int) ([]models.AuditEntry, error) {
	const op = "listar auditoria"
	rows, err := q.QueryContext(ctx,
		`SELECT a.id, u.name, au.name, a.action, a.entity, a.entity_id, a.before_json, a.after_json, a.created_at
		 FROM audit_log a LEFT JOIN users u ON u.id = a.user_id LEFT JOIN users au ON au.id = a.authorized_by_id
		 WHERE a.created_at >= ? AND a.created_at < ? ORDER BY a.created_at DESC, a.id DESC LIMIT ?`,
		FormatTime(start), FormatTime(end), limit)
	if err != nil {
		return nil, wrap(op, err)
	}
	defer rows.Close()
	out := []models.AuditEntry{}
	for rows.Next() {
		var (
			e                  models.AuditEntry
			user, auth, entity sql.NullString
			entityID           sql.NullInt64
			before, after      sql.NullString
			created            string
		)
		if err := rows.Scan(&e.ID, &user, &auth, &e.Action, &entity, &entityID, &before, &after, &created); err != nil {
			return nil, wrap(op, err)
		}
		e.UserName, e.AuthorizedBy, e.Entity, e.EntityID = nullStr(user), nullStr(auth), nullStr(entity), nullInt(entityID)
		e.BeforeJSON, e.AfterJSON = nullStr(before), nullStr(after)
		if e.CreatedAt, err = parseTime(created); err != nil {
			return nil, wrap(op, err)
		}
		out = append(out, e)
	}
	return out, wrap(op, rows.Err())
}
