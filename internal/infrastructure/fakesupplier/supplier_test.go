package fakesupplier

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/domain"
)

func TestScenarios(t *testing.T) {
	for _, name := range []string{"basic", "repeat", "late-old", "bad-data", "outage"} {
		t.Run(name, func(t *testing.T) {
			s, err := New(name, 0)
			if err != nil {
				t.Fatal(err)
			}
			cursor := ""
			count := 0
			for {
				b, err := s.FetchBatch(context.Background(), cursor)
				if err != nil {
					t.Fatal(err)
				}
				count++
				if b.ID == "" || len(b.Records) == 0 {
					t.Fatal(b)
				}
				cursor = b.NextCursor
				if cursor == "" {
					break
				}
				if count > 10 {
					t.Fatal("unbounded scenario")
				}
			}
		})
	}
}
func TestOutageRecovers(t *testing.T) {
	s, err := New("outage", 2)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.FetchBatch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FetchBatch(context.Background(), b.NextCursor); err == nil {
		t.Fatal("missing outage")
	}
	if _, err := s.FetchBatch(context.Background(), b.NextCursor); err != nil {
		t.Fatal(err)
	}
}
func TestBadInput(t *testing.T) {
	if _, err := New("../basic", 0); err == nil {
		t.Fatal("accepted path")
	}
	s, _ := New("basic", 0)
	for _, cursor := range []string{"x", "-1", "99"} {
		if _, err := s.FetchBatch(context.Background(), cursor); err == nil {
			t.Fatal(cursor)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.FetchBatch(ctx, ""); err == nil {
		t.Fatal("ignored cancel")
	}
}

func TestCursorSurvivesSupplierRestart(t *testing.T) {
	s, err := New("outage", 4)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	cursor := ""
	for i := 0; i < 3; i++ {
		b, err := s.FetchBatch(ctx, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(b.NextCursor, "cur_") {
			t.Fatal("expected supplier token", b.NextCursor)
		}
		cursor = b.NextCursor
	}
	if _, err := s.FetchBatch(ctx, cursor); err == nil {
		t.Fatal("expected outage at batch 4")
	}
	restarted, err := New("outage", 0)
	if err != nil {
		t.Fatal(err)
	}
	b, err := restarted.FetchBatch(ctx, cursor)
	if err != nil || b.ID != "outage-4" {
		t.Fatal(b, err)
	}
	if _, err := restarted.FetchBatch(ctx, "3"); err == nil {
		t.Fatal("outage scenario accepted a numeric cursor")
	}
	if _, err := restarted.FetchBatch(ctx, cursor+"x"); err == nil {
		t.Fatal("accepted modified cursor")
	}
	other, _ := New("basic", 0)
	if _, err := other.FetchBatch(ctx, cursor); err == nil {
		t.Fatal("accepted another scenario's token")
	}
}

func TestLegacyNumericCheckpoint(t *testing.T) {
	for _, name := range []string{"basic", "repeat", "late-old", "bad-data"} {
		s, err := New(name, 0)
		if err != nil {
			t.Fatal(err)
		}
		b, err := s.FetchBatch(context.Background(), "1")
		if err != nil || b.ID == "" {
			t.Fatal(name, b, err)
		}
		if b.NextCursor != "" && !strings.HasPrefix(b.NextCursor, "cur_") {
			t.Fatal("did not advance to a token", b)
		}
	}
}

func TestOutageCatalogReconciliation(t *testing.T) {
	s, err := New("outage", 0)
	if err != nil {
		t.Fatal(err)
	}
	products := map[string]domain.Product{}
	cursor := ""
	batches, records, invalid := 0, 0, 0
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for {
		b, err := s.FetchBatch(context.Background(), cursor)
		if err != nil {
			t.Fatal(err)
		}
		batches++
		records += len(b.Records)
		for _, r := range b.Records {
			if r.Validate() != nil {
				invalid++
				continue
			}
			var old *domain.Product
			if p, exists := products[r.SKU]; exists {
				old = &p
			}
			p, _ := domain.Reconcile(old, r, b.ID, now.Add(time.Duration(batches)*time.Second))
			products[r.SKU] = p
		}
		cursor = b.NextCursor
		if cursor == "" {
			break
		}
	}
	active, withdrawn, created, updated := 0, 0, 0, 0
	for _, p := range products {
		if p.Status == "withdrawn" {
			withdrawn++
			continue
		}
		active++
		if p.PreviousBatchID == "" {
			created++
		} else {
			updated++
		}
	}
	if batches != 6 || records != 121 || invalid != 3 || len(products) != 41 || active != 37 || withdrawn != 4 || created != 25 || updated != 12 {
		t.Fatalf("batches=%d records=%d invalid=%d products=%d active=%d withdrawn=%d created=%d updated=%d", batches, records, invalid, len(products), active, withdrawn, created, updated)
	}
	if products["OLD-001"].Version != 5 || products["OLD-001"].Status != "withdrawn" {
		t.Fatal("old active record revived tombstone")
	}
	if products["SHOP-008"].Version != 4 || products["SHOP-008"].Status != "active" {
		t.Fatal("late withdrawal replaced newer re-listing")
	}
	if products["SHOP-026"].BatchID != "outage-2" || products["SHOP-026"].Version != 2 {
		t.Fatal("version-only change moved metadata")
	}
}
