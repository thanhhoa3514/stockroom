package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/thanhhoa3514/stockroom/internal/application"
	"github.com/thanhhoa3514/stockroom/internal/domain"
)

type runs struct {
	tx  *sql.Tx
	ctx context.Context
}

const runColumns = "id,status,cursor,started_at,finished_at,error,invalid_count,issues"

func scanRun(row scanner) (domain.SyncRun, error) {
	var r domain.SyncRun
	var start, issues string
	var end sql.NullString
	err := row.Scan(&r.ID, &r.Status, &r.Cursor, &start, &end, &r.Error, &r.InvalidCount, &issues)
	if errors.Is(err, sql.ErrNoRows) {
		return r, application.ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.StartedAt, err = parse(start)
	if err != nil {
		return r, err
	}
	if end.Valid {
		r.FinishedAt, err = parse(end.String)
		if err != nil {
			return r, err
		}
	}
	err = json.Unmarshal([]byte(issues), &r.Issues)
	return r, err
}
func issueJSON(r domain.SyncRun) (string, error) {
	issues := r.Issues
	if len(issues) > 100 {
		issues = issues[:100]
	}
	if issues == nil {
		issues = []domain.Issue{}
	}
	b, err := json.Marshal(issues)
	return string(b), err
}
func (r runs) Create(v *domain.SyncRun) error {
	issues, err := issueJSON(*v)
	if err != nil {
		return err
	}
	var end any
	if !v.FinishedAt.IsZero() {
		end = format(v.FinishedAt)
	}
	res, err := r.tx.ExecContext(r.ctx, "INSERT INTO sync_runs(status,cursor,started_at,finished_at,error,invalid_count,issues) VALUES (?,?,?,?,?,?,?)", v.Status, v.Cursor, format(v.StartedAt), end, v.Error, v.InvalidCount, issues)
	if err != nil {
		return err
	}
	v.ID, err = res.LastInsertId()
	return err
}
func (r runs) Get(id int64) (domain.SyncRun, error) {
	return scanRun(r.tx.QueryRowContext(r.ctx, "SELECT "+runColumns+" FROM sync_runs WHERE id=?", id))
}
func (r runs) Latest() (domain.SyncRun, error) {
	return scanRun(r.tx.QueryRowContext(r.ctx, "SELECT "+runColumns+" FROM sync_runs ORDER BY id DESC LIMIT 1"))
}
func (r runs) Save(v domain.SyncRun) error {
	issues, err := issueJSON(v)
	if err != nil {
		return err
	}
	var end any
	if !v.FinishedAt.IsZero() {
		end = format(v.FinishedAt)
	}
	res, err := r.tx.ExecContext(r.ctx, "UPDATE sync_runs SET status=?,cursor=?,started_at=?,finished_at=?,error=?,invalid_count=?,issues=? WHERE id=?", v.Status, v.Cursor, format(v.StartedAt), end, v.Error, v.InvalidCount, issues, v.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return application.ErrNotFound
	}
	return err
}
func (r runs) FindByStatus(status string) ([]domain.SyncRun, error) {
	rows, err := r.tx.QueryContext(r.ctx, "SELECT "+runColumns+" FROM sync_runs WHERE status=? ORDER BY id DESC", status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SyncRun{}
	for rows.Next() {
		v, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
