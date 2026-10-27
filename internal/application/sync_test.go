package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/domain"
)

func setup() (*memory, *fakeSupplier, *SyncService) {
	m := newMemory()
	s := &fakeSupplier{batches: map[string]domain.Batch{
		"":  {ID: "one", Records: []domain.Record{{SKU: "a", Name: "Apple", Version: 1}}, NextCursor: "1"},
		"1": {ID: "two", Records: []domain.Record{{SKU: "b", Name: "Banana", Version: 1}}},
	}}
	return m, s, &SyncService{m, s, &fakeClock{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}}
}
func TestFullSyncAndRepeat(t *testing.T) {
	m, _, svc := setup()
	for i := 0; i < 2; i++ {
		out, err := (StartSync{svc}).Execute(context.Background(), SyncInput{})
		if err != nil {
			t.Fatal(err)
		}
		status, err := (GetSyncStatus{m, svc.Clock}).Execute(context.Background(), StatusInput{out.RunID})
		if err != nil || status.Status != "completed" {
			t.Fatalf("%+v %v", status, err)
		}
		want := 2
		if i == 1 {
			want = 0
		}
		if status.Counts.Created != want {
			t.Fatal(status.Counts)
		}
	}
	if len(m.products) != 2 {
		t.Fatal(m.products)
	}
}
func TestFailureAndResume(t *testing.T) {
	m, s, svc := setup()
	s.fail = "1"
	s.err = errors.New("offline")
	_, err := (StartSync{svc}).Execute(context.Background(), SyncInput{})
	if err == nil {
		t.Fatal("expected error")
	}
	p := m.products["a"]
	if len(m.products) != 1 || m.runs[1].Cursor != "1" {
		t.Fatal(m)
	}
	status, err := (GetSyncStatus{m, svc.Clock}).Execute(context.Background(), StatusInput{})
	if err != nil || status.Status != "incomplete" {
		t.Fatal(status, err)
	}
	if _, err := (StartSync{svc}).Execute(context.Background(), SyncInput{}); err == nil {
		t.Fatal("start accepted unfinished run")
	}
	s.err = nil
	s.cursors = nil
	if _, err := (ResumeSync{svc}).Execute(context.Background(), SyncInput{}); err != nil {
		t.Fatal(err)
	}
	if s.cursors[0] != "1" || m.runs[1].Status != "completed" || m.products["a"] != p {
		t.Fatal("resume lost completed work")
	}
}
func TestInitialFailureKeepsCatalog(t *testing.T) {
	m, s, svc := setup()
	m.products["old"] = domain.Product{SKU: "old", Version: 9}
	s.err = errors.New("offline")
	_, err := (StartSync{svc}).Execute(context.Background(), SyncInput{})
	if err == nil || len(m.products) != 1 || m.products["old"].Version != 9 {
		t.Fatal(m, err)
	}
}
func TestCancellationFinishesBatch(t *testing.T) {
	m, s, svc := setup()
	ctx, cancel := context.WithCancel(context.Background())
	s.afterFetch = cancel
	_, err := (StartSync{svc}).Execute(ctx, SyncInput{})
	if !errors.Is(err, context.Canceled) || len(m.products) != 1 || m.runs[1].Cursor != "1" || m.runs[1].Status != "incomplete" {
		t.Fatal(m, err)
	}
}
func TestRecover(t *testing.T) {
	m, _, svc := setup()
	m.runs[1] = domain.SyncRun{ID: 1, Status: "running"}
	if err := svc.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.runs[1].Status != "incomplete" {
		t.Fatal(m.runs)
	}
}
func TestInvalidRecords(t *testing.T) {
	m, s, svc := setup()
	b := s.batches[""]
	b.Records = append(b.Records, domain.Record{})
	s.batches[""] = b
	if _, err := (StartSync{svc}).Execute(context.Background(), SyncInput{}); err != nil {
		t.Fatal(err)
	}
	if m.runs[1].InvalidCount != 1 || len(m.runs[1].Issues) != 1 || len(m.products) != 2 {
		t.Fatal(m)
	}
}

func TestVersionOnlyRunHasNoChanges(t *testing.T) {
	m, s, svc := setup()
	ctx := context.Background()
	if _, err := (StartSync{svc}).Execute(ctx, SyncInput{}); err != nil {
		t.Fatal(err)
	}
	before := m.products["a"]
	s.batches = map[string]domain.Batch{"": {ID: "new-batch", Records: []domain.Record{{SKU: "a", Name: "Apple", Version: 2}}}}
	if _, err := (StartSync{svc}).Execute(ctx, SyncInput{}); err != nil {
		t.Fatal(err)
	}
	status, err := (GetSyncStatus{m, svc.Clock}).Execute(ctx, StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Counts != (Counts{}) {
		t.Fatalf("version-only run reported changes: %+v", status)
	}
	after := m.products["a"]
	want := before
	want.Version = 2
	if after != want {
		t.Fatalf("metadata changed: %+v", after)
	}
	ps, err := (ListProducts{m}).Execute(ctx, ProductsInput{Status: "active"})
	if err != nil || len(ps) != 2 {
		t.Fatal(ps, err)
	}
	for _, p := range ps {
		if p.SKU == "a" && !p.CreatedAt.Equal(before.CreatedAt) {
			t.Fatal("DTO lost created_at")
		}
	}
}

func TestSameSKURecordsInOrderAndAbsentProduct(t *testing.T) {
	m, s, svc := setup()
	absent := domain.Product{SKU: "absent", Name: "Keep", Status: "active", Version: 8}
	m.products[absent.SKU] = absent
	s.batches = map[string]domain.Batch{"": {ID: "ordered", Records: []domain.Record{
		{SKU: "a", Name: "Apple", Version: 1},
		{SKU: "a", Withdrawn: true, Version: 3},
		{SKU: "a", Name: "Old", Version: 2},
		{SKU: "a", Name: "Relisted", PriceCents: 20, Version: 4},
	}}}
	if _, err := (StartSync{svc}).Execute(context.Background(), SyncInput{}); err != nil {
		t.Fatal(err)
	}
	p := m.products["a"]
	if p.Version != 4 || p.Name != "Relisted" || p.Status != "active" || p.PriceCents != 20 {
		t.Fatal(p)
	}
	if m.products["absent"] != absent {
		t.Fatal("absent product was changed")
	}
}

func TestIssuesCapAcrossResume(t *testing.T) {
	m, s, svc := setup()
	for _, cursor := range []string{"", "1"} {
		b := s.batches[cursor]
		b.Records = append(b.Records, make([]domain.Record, 75)...)
		s.batches[cursor] = b
	}
	s.fail = "1"
	s.err = errors.New("offline")
	if _, err := (StartSync{svc}).Execute(context.Background(), SyncInput{}); err == nil {
		t.Fatal("expected outage")
	}
	s.err = nil
	if _, err := (ResumeSync{svc}).Execute(context.Background(), SyncInput{}); err != nil {
		t.Fatal(err)
	}
	status, err := (GetSyncStatus{m, svc.Clock}).Execute(context.Background(), StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "completed" || status.InvalidCount != 150 || len(status.Issues) != 100 {
		t.Fatal(status)
	}
	if status.Issues[74].BatchID != "one" || status.Issues[75].BatchID != "two" {
		t.Fatal("first 100 issues were not retained in order")
	}
}

func TestRunningRunIsNotReportedComplete(t *testing.T) {
	m, _, svc := setup()
	m.next = 1
	m.runs[1] = domain.SyncRun{ID: 1, Status: "running", Cursor: "1", StartedAt: svc.Clock.Now()}
	status, err := (GetSyncStatus{m, svc.Clock}).Execute(context.Background(), StatusInput{RunID: 1})
	if err != nil || status.Status != "running" || !status.FinishedAt.IsZero() {
		t.Fatal(status, err)
	}
	if _, err := (StartSync{svc}).Execute(context.Background(), SyncInput{}); err == nil {
		t.Fatal("start accepted running run")
	}
}
