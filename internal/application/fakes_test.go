package application

import (
	"context"
	"sort"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/domain"
)

type memory struct {
	products map[string]domain.Product
	runs     map[int64]domain.SyncRun
	next     int64
}

func newMemory() *memory {
	return &memory{products: map[string]domain.Product{}, runs: map[int64]domain.SyncRun{}}
}
func (m *memory) Do(ctx context.Context, fn func(Repositories) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	copy := newMemory()
	copy.next = m.next
	for k, v := range m.products {
		copy.products[k] = v
	}
	for k, v := range m.runs {
		v.Issues = append([]domain.Issue(nil), v.Issues...)
		copy.runs[k] = v
	}
	if err := fn(Repositories{productMemory{copy}, runMemory{copy}}); err != nil {
		return err
	}
	*m = *copy
	return nil
}

type productMemory struct{ m *memory }

func (p productMemory) Get(s string) (*domain.Product, error) {
	v, ok := p.m.products[s]
	if !ok {
		return nil, ErrNotFound
	}
	return &v, nil
}
func (p productMemory) Save(v domain.Product) error { p.m.products[v.SKU] = v; return nil }
func (p productMemory) List(s string) ([]domain.Product, error) {
	out := []domain.Product{}
	for _, v := range p.m.products {
		if s == "" || v.Status == s {
			out = append(out, v)
		}
	}
	return out, nil
}
func (p productMemory) SummarizeRun(from, to time.Time) (Counts, error) {
	var c Counts
	for _, v := range p.m.products {
		if v.UpdatedAt.Before(from) || v.UpdatedAt.After(to) {
			continue
		}
		if v.Status == "withdrawn" {
			c.Withdrawn++
		} else if v.PreviousBatchID == "" {
			c.Created++
		} else {
			c.Updated++
		}
	}
	return c, nil
}

type runMemory struct{ m *memory }

func (r runMemory) Create(v *domain.SyncRun) error {
	r.m.next++
	v.ID = r.m.next
	r.m.runs[v.ID] = *v
	return nil
}
func (r runMemory) Get(id int64) (domain.SyncRun, error) {
	v, ok := r.m.runs[id]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (r runMemory) Latest() (domain.SyncRun, error) { return r.Get(r.m.next) }
func (r runMemory) Save(v domain.SyncRun) error     { r.m.runs[v.ID] = v; return nil }
func (r runMemory) FindByStatus(s string) ([]domain.SyncRun, error) {
	out := []domain.SyncRun{}
	for _, v := range r.m.runs {
		if v.Status == s {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { c.now = c.now.Add(time.Second); return c.now }

type fakeSupplier struct {
	batches    map[string]domain.Batch
	fail       string
	err        error
	cursors    []string
	afterFetch func()
}

func (s *fakeSupplier) FetchBatch(_ context.Context, cursor string) (domain.Batch, error) {
	s.cursors = append(s.cursors, cursor)
	if cursor == s.fail && s.err != nil {
		return domain.Batch{}, s.err
	}
	if s.afterFetch != nil {
		s.afterFetch()
	}
	return s.batches[cursor], nil
}
