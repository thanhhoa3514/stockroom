package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/application"
	_ "modernc.org/sqlite"
)

//go:embed 001_init.sql
var migration string

//go:embed 002_product_created_at.sql
var createdAtMigration string

type Database struct{ db *sql.DB }

func Open(path string) (*Database, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = db.Exec(stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize database: %w", err)
		}
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return &Database{db}, nil
}

func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(migration); err != nil {
		return err
	}
	rows, err := tx.Query("PRAGMA table_info(products)")
	if err != nil {
		return err
	}
	hasCreatedAt := false
	for rows.Next() {
		var index, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if name == "created_at" {
			hasCreatedAt = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// Inspect the schema so existing databases upgrade without a third tracking table.
	if !hasCreatedAt {
		if _, err := tx.Exec(createdAtMigration); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (d *Database) Close() error { return d.db.Close() }
func (d *Database) Do(ctx context.Context, fn func(application.Repositories) error) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	repos := application.Repositories{Products: products{tx, ctx}, Runs: runs{tx, ctx}}
	if err := fn(repos); err != nil {
		return err
	}
	return tx.Commit()
}

// Fixed fractional precision keeps text timestamps ordered even across second boundaries.
const timestamp = "2006-01-02T15:04:05.000000000Z"

func format(t time.Time) string         { return t.UTC().Format(timestamp) }
func parse(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
