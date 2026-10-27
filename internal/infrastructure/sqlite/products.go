package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/application"
	"github.com/thanhhoa3514/stockroom/internal/domain"
)

type products struct {
	tx  *sql.Tx
	ctx context.Context
}
type scanner interface{ Scan(...any) error }

const productColumns = "sku,name,price_cents,status,version,batch_id,previous_batch_id,created_at,updated_at"

func scanProduct(row scanner) (domain.Product, error) {
	var p domain.Product
	var created, at string
	err := row.Scan(&p.SKU, &p.Name, &p.PriceCents, &p.Status, &p.Version, &p.BatchID, &p.PreviousBatchID, &created, &at)
	if err != nil {
		return p, err
	}
	p.CreatedAt, err = parse(created)
	if err != nil {
		return p, err
	}
	p.UpdatedAt, err = parse(at)
	return p, err
}
func (r products) Get(sku string) (*domain.Product, error) {
	p, err := scanProduct(r.tx.QueryRowContext(r.ctx, "SELECT "+productColumns+" FROM products WHERE sku=?", sku))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, application.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func (r products) Save(p domain.Product) error {
	_, err := r.tx.ExecContext(r.ctx, `INSERT INTO products (`+productColumns+`) VALUES (?,?,?,?,?,?,?,?,?)
 ON CONFLICT(sku) DO UPDATE SET name=excluded.name,price_cents=excluded.price_cents,status=excluded.status,version=excluded.version,batch_id=excluded.batch_id,previous_batch_id=excluded.previous_batch_id,updated_at=excluded.updated_at`, p.SKU, p.Name, p.PriceCents, p.Status, p.Version, p.BatchID, p.PreviousBatchID, format(p.CreatedAt), format(p.UpdatedAt))
	return err
}
func (r products) List(status string) ([]domain.Product, error) {
	rows, err := r.tx.QueryContext(r.ctx, "SELECT "+productColumns+" FROM products WHERE (?='' OR status=?) ORDER BY sku", status, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Product{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (r products) SummarizeRun(from, to time.Time) (application.Counts, error) {
	var c application.Counts
	err := r.tx.QueryRowContext(r.ctx, `SELECT
 COALESCE(SUM(CASE WHEN status='active' AND previous_batch_id='' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN status='active' AND previous_batch_id<>'' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN status='withdrawn' THEN 1 ELSE 0 END),0)
 FROM products WHERE updated_at>=? AND updated_at<=?`, format(from), format(to)).Scan(&c.Created, &c.Updated, &c.Withdrawn)
	return c, err
}
