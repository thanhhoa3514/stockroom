package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/application"
	"github.com/thanhhoa3514/stockroom/internal/domain"
)

func openTest(t *testing.T) *Database {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
func TestMigrationAndUpsert(t *testing.T) {
	d := openTest(t)
	var count int
	if err := d.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	for _, pragma := range []struct{ name, want string }{{"journal_mode", "wal"}, {"busy_timeout", "5000"}, {"foreign_keys", "1"}} {
		var got string
		if err := d.db.QueryRow("PRAGMA " + pragma.name).Scan(&got); err != nil || got != pragma.want {
			t.Fatal(pragma, got, err)
		}
	}
	err := d.Do(context.Background(), func(r application.Repositories) error {
		now := time.Now()
		p, _ := domain.Reconcile(nil, domain.Record{SKU: "x", Withdrawn: true, Version: 3}, "batch", now)
		if err := r.Products.Save(p); err != nil {
			return err
		}
		old, err := r.Products.Get("x")
		if err != nil {
			return err
		}
		p, out := domain.Reconcile(old, domain.Record{SKU: "x", Name: "Old", Version: 2}, "late", now)
		if out != domain.Stale {
			t.Fatal(out)
		}
		if err := r.Products.Save(p); err != nil {
			return err
		}
		ps, err := r.Products.List("withdrawn")
		if err == nil && (len(ps) != 1 || ps[0].Name != "" || ps[0].Version != 3) {
			t.Fatal(ps)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing", "db")); err == nil {
		t.Fatal("opened missing parent")
	}
}
func TestRollbackWholeBatch(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	run := domain.SyncRun{Status: "running", StartedAt: time.Now()}
	if err := d.Do(ctx, func(r application.Repositories) error { return r.Runs.Create(&run) }); err != nil {
		t.Fatal(err)
	}
	err := d.Do(ctx, func(r application.Repositories) error {
		p, _ := domain.Reconcile(nil, domain.Record{SKU: "good", Name: "A", Version: 1}, "b", time.Now())
		if err := r.Products.Save(p); err != nil {
			return err
		}
		run.Advance("2", nil)
		if err := r.Runs.Save(run); err != nil {
			return err
		}
		p.SKU = "bad"
		p.Status = "invalid"
		return r.Products.Save(p)
	})
	if err == nil {
		t.Fatal("expected constraint failure")
	}
	if err := d.Do(ctx, func(r application.Repositories) error {
		ps, err := r.Products.List("")
		if err != nil {
			return err
		}
		saved, err := r.Runs.Get(run.ID)
		if len(ps) != 0 || saved.Cursor != "" {
			t.Fatal("partial commit", ps, saved)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func TestUniqueRunningRun(t *testing.T) {
	d := openTest(t)
	err := d.Do(context.Background(), func(r application.Repositories) error {
		a := domain.SyncRun{Status: "running", StartedAt: time.Now()}
		if err := r.Runs.Create(&a); err != nil {
			return err
		}
		b := a
		return r.Runs.Create(&b)
	})
	if err == nil {
		t.Fatal("two running runs accepted")
	}
}

type testClock struct{ now time.Time }

func (c *testClock) Now() time.Time { c.now = c.now.Add(time.Second); return c.now }

type crashSupplier struct {
	db      *Database
	crash   bool
	cursors []string
}

func (s *crashSupplier) FetchBatch(_ context.Context, cursor string) (domain.Batch, error) {
	s.cursors = append(s.cursors, cursor)
	if s.crash && cursor == "2" {
		s.db.Close()
		return domain.Batch{}, errors.New("simulated process death")
	}
	switch cursor {
	case "":
		return domain.Batch{ID: "b1", Records: []domain.Record{{SKU: "a", Name: "A", Version: 1}}, NextCursor: "1"}, nil
	case "1":
		return domain.Batch{ID: "b2", Records: []domain.Record{{SKU: "b", Name: "B", Version: 1}}, NextCursor: "2"}, nil
	default:
		return domain.Batch{ID: "b3", Records: []domain.Record{{SKU: "c", Name: "C", Version: 1}}}, nil
	}
}
func TestCrashAfterBatchTwo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crash.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{time.Now()}
	supplier := &crashSupplier{db: d, crash: true}
	svc := &application.SyncService{UOW: d, Supplier: supplier, Clock: clock}
	ctx := context.Background()
	if _, err := (application.StartSync{Service: svc}).Execute(ctx, application.SyncInput{}); err == nil {
		t.Fatal("expected crash")
	}
	d, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	svc.UOW = d
	supplier.crash = false
	supplier.cursors = nil
	if err := d.Do(ctx, func(r application.Repositories) error {
		run, err := r.Runs.Latest()
		if err != nil {
			return err
		}
		ps, err := r.Products.List("")
		if run.Status != "running" || run.Cursor != "2" || len(ps) != 2 {
			t.Fatal(run, ps)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.ResumeSync{Service: svc}).Execute(ctx, application.SyncInput{}); err != nil {
		t.Fatal(err)
	}
	if len(supplier.cursors) != 1 || supplier.cursors[0] != "2" {
		t.Fatal(supplier.cursors)
	}
	if err := d.Do(ctx, func(r application.Repositories) error {
		run, err := r.Runs.Latest()
		if err != nil {
			return err
		}
		ps, err := r.Products.List("")
		if run.Status != "completed" || len(ps) != 3 {
			t.Fatal(run, ps)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradeLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the schema from before created_at was introduced.
	legacy := strings.Replace(migration, " created_at TEXT NOT NULL,\n", "", 1)
	if _, err := raw.Exec(legacy); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	at := "2026-01-01T10:00:00.123456789Z"
	if _, err := raw.Exec(`INSERT INTO products(sku,name,price_cents,status,version,batch_id,previous_batch_id,updated_at) VALUES ('old','Old',100,'active',3,'b3','b2',?)`, at); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		d, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		err = d.Do(context.Background(), func(r application.Repositories) error {
			p, err := r.Products.Get("old")
			if err != nil {
				return err
			}
			if format(p.CreatedAt) != at || format(p.UpdatedAt) != at || p.Version != 3 || p.PreviousBatchID != "b2" {
				t.Fatalf("legacy row damaged: %+v", p)
			}
			return nil
		})
		d.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreatedAtPersistsAcrossChanges(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	created := time.Date(2026, 1, 1, 0, 0, 0, 123, time.UTC)
	err := d.Do(ctx, func(r application.Repositories) error {
		var old *domain.Product
		for i, record := range []domain.Record{
			{SKU: "x", Withdrawn: true, Version: 1},
			{SKU: "x", Name: "X", PriceCents: 10, Version: 2},
			{SKU: "x", Name: "X", PriceCents: 10, Version: 3},
			{SKU: "x", Withdrawn: true, Version: 4},
		} {
			p, _ := domain.Reconcile(old, record, fmt.Sprintf("b%d", i), created.Add(time.Duration(i)*time.Second))
			if err := r.Products.Save(p); err != nil {
				return err
			}
			var err error
			old, err = r.Products.Get("x")
			if err != nil {
				return err
			}
			if !old.CreatedAt.Equal(created) {
				t.Fatal("creation time changed", old)
			}
		}
		// Repository upserts also protect the persisted creation time.
		old.CreatedAt = created.Add(time.Hour)
		if err := r.Products.Save(*old); err != nil {
			return err
		}
		ps, err := r.Products.List("withdrawn")
		if err != nil {
			return err
		}
		if len(ps) != 1 || !ps[0].CreatedAt.Equal(created) {
			t.Fatal(ps)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunSummarySubsecondWindowsAndVersionOnlyChange(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	err := d.Do(ctx, func(r application.Repositories) error {
		p, _ := domain.Reconcile(nil, domain.Record{SKU: "a", Name: "A", Version: 1}, "first", base.Add(100*time.Nanosecond))
		if err := r.Products.Save(p); err != nil {
			return err
		}
		p, out := domain.Reconcile(&p, domain.Record{SKU: "a", Name: "A", Version: 2}, "second", base.Add(300*time.Nanosecond))
		if out != domain.Unchanged {
			t.Fatal(out)
		}
		if err := r.Products.Save(p); err != nil {
			return err
		}
		first, err := r.Products.SummarizeRun(base, base.Add(200*time.Nanosecond))
		if err != nil {
			return err
		}
		second, err := r.Products.SummarizeRun(base.Add(201*time.Nanosecond), base.Add(400*time.Nanosecond))
		if err != nil {
			return err
		}
		if first.Created != 1 || second != (application.Counts{}) {
			t.Fatal(first, second)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunIssuesRoundTrip(t *testing.T) {
	d := openTest(t)
	ctx := context.Background()
	run := domain.SyncRun{Status: "running", StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	issues := make([]domain.Issue, 150)
	for i := range issues {
		issues[i] = domain.Issue{BatchID: "b1", SKU: fmt.Sprintf("bad-%d", i), Reason: "invalid version"}
	}
	run.Advance("1", issues)
	run.MarkIncomplete(run.StartedAt.Add(time.Second), "offline")
	if err := d.Do(ctx, func(r application.Repositories) error { return r.Runs.Create(&run) }); err != nil {
		t.Fatal(err)
	}
	if err := d.Do(ctx, func(r application.Repositories) error {
		saved, err := r.Runs.Get(run.ID)
		if err != nil {
			return err
		}
		if saved.InvalidCount != 150 || len(saved.Issues) != 100 || saved.Issues[99].SKU != "bad-99" || saved.Status != "incomplete" || saved.Cursor != "1" {
			t.Fatal(saved)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
