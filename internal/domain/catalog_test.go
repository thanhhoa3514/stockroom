package domain

import (
	"testing"
	"time"
)

func TestReconcile(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	active := Product{SKU: "a", Name: "Apple", PriceCents: 10, Status: "active", Version: 2, BatchID: "old", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour)}
	tomb := Product{SKU: "a", Status: "withdrawn", Version: 3, BatchID: "gone", CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-time.Hour)}
	tests := []struct {
		name    string
		old     *Product
		in      Record
		want    Outcome
		status  string
		version int64
	}{
		{"create", nil, Record{SKU: "a", Name: "Apple", Version: 1}, Created, "active", 1},
		{"update", &active, Record{SKU: "a", Name: "Pear", Version: 3}, Updated, "active", 3},
		{"withdraw", &active, Record{SKU: "a", Withdrawn: true, Version: 3}, Withdrawn, "withdrawn", 3},
		{"unchanged", &active, Record{SKU: "a", Name: "Apple", PriceCents: 10, Version: 3}, Unchanged, "active", 3},
		{"equal", &active, Record{SKU: "a", Version: 2}, Stale, "active", 2},
		{"equal withdrawal", &active, Record{SKU: "a", Withdrawn: true, Version: 2}, Stale, "active", 2},
		{"lower", &active, Record{SKU: "a", Version: 1}, Stale, "active", 2},
		{"tombstone", nil, Record{SKU: "a", Name: "Ignored", PriceCents: 999, Withdrawn: true, Version: 3}, Created, "withdrawn", 3},
		{"blocks revival", &tomb, Record{SKU: "a", Name: "Apple", Version: 2}, Stale, "withdrawn", 3},
		{"relist", &tomb, Record{SKU: "a", Name: "Apple", Version: 4}, Updated, "active", 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, out := Reconcile(tt.old, tt.in, "new", now)
			if out != tt.want || p.Status != tt.status || p.Version != tt.version {
				t.Fatalf("got %+v %s", p, out)
			}
			if out == Stale {
				if p != *tt.old {
					t.Fatal("stale changed product")
				}
			} else if out == Unchanged {
				want := *tt.old
				want.Version = tt.in.Version
				if p != want {
					t.Fatalf("version-only update changed metadata: got %+v want %+v", p, want)
				}
			} else {
				if p.BatchID != "new" || p.UpdatedAt != now {
					t.Fatal("missing metadata")
				}
				if tt.old != nil && p.PreviousBatchID != tt.old.BatchID {
					t.Fatal("missing previous batch")
				}
			}
			if tt.old == nil && p.CreatedAt != now {
				t.Fatal("missing creation time")
			}
			if tt.old != nil && p.CreatedAt != tt.old.CreatedAt {
				t.Fatal("creation time changed")
			}
			if tt.in.Withdrawn && tt.old != nil && (p.Name != tt.old.Name || p.PriceCents != tt.old.PriceCents) {
				t.Fatal("withdrawal lost name or price")
			}
			if tt.in.Withdrawn && tt.old == nil && (p.Name != "" || p.PriceCents != 0) {
				t.Fatal("unknown withdrawal must create an empty tombstone")
			}
		})
	}
	if active.Version != 2 {
		t.Fatal("input mutated; absent products must remain untouched")
	}
}
func TestValidate(t *testing.T) {
	for _, r := range []Record{{}, {SKU: "a", Version: 0}, {SKU: "a", Version: 1}, {SKU: "a", Name: "A", PriceCents: -1, Version: 1}} {
		if r.Validate() == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
	for _, r := range []Record{{SKU: "a", Name: "A", Version: 1}, {SKU: "a", Withdrawn: true, Version: 1, PriceCents: -1}} {
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestRun(t *testing.T) {
	r := SyncRun{Status: "running"}
	r.Complete(time.Time{})
	if r.Status != "running" {
		t.Fatal("completed before terminal batch")
	}
	issues := make([]Issue, 120)
	r.Advance("next", issues)
	r.Complete(time.Time{})
	if r.Status != "running" || len(r.Issues) != 100 || r.InvalidCount != 120 {
		t.Fatal(r)
	}
	r.Advance("", nil)
	r.Complete(time.Time{})
	if r.Status != "completed" {
		t.Fatal(r)
	}
}
