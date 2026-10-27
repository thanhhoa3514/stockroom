package application

import (
	"context"
	"errors"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/domain"
)

var ErrNotFound = errors.New("not found")

type Supplier interface {
	FetchBatch(context.Context, string) (domain.Batch, error)
}
type Clock interface{ Now() time.Time }
type UnitOfWork interface {
	Do(context.Context, func(Repositories) error) error
}
type Repositories struct {
	Products ProductRepository
	Runs     SyncRunRepository
}
type ProductRepository interface {
	Get(string) (*domain.Product, error)
	Save(domain.Product) error
	List(string) ([]domain.Product, error)
	SummarizeRun(time.Time, time.Time) (Counts, error)
}
type SyncRunRepository interface {
	Create(*domain.SyncRun) error
	Get(int64) (domain.SyncRun, error)
	Latest() (domain.SyncRun, error)
	Save(domain.SyncRun) error
	FindByStatus(string) ([]domain.SyncRun, error)
}
