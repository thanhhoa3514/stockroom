package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/domain"
)

type SyncService struct {
	UOW      UnitOfWork
	Supplier Supplier
	Clock    Clock
}
type StartSync struct{ Service *SyncService }
type ResumeSync struct{ Service *SyncService }

// Recover is called once at process startup. Only one CLI process may update a database.
func (s *SyncService) Recover(ctx context.Context) error {
	return s.UOW.Do(ctx, func(r Repositories) error {
		runs, err := r.Runs.FindByStatus("running")
		if err != nil {
			return err
		}
		for _, run := range runs {
			run.MarkIncomplete(s.Clock.Now(), "previous process stopped; resume required")
			if err := r.Runs.Save(run); err != nil {
				return err
			}
		}
		return nil
	})
}
func (u StartSync) Execute(ctx context.Context, _ SyncInput) (SyncOutput, error) {
	s := u.Service
	run := domain.SyncRun{Status: "running", StartedAt: s.Clock.Now()}
	err := s.UOW.Do(ctx, func(r Repositories) error {
		for _, state := range []string{"running", "incomplete"} {
			runs, err := r.Runs.FindByStatus(state)
			if err != nil {
				return err
			}
			if len(runs) > 0 {
				return errors.New("unfinished sync exists; use stockroom resume")
			}
		}
		return r.Runs.Create(&run)
	})
	if err != nil {
		return SyncOutput{}, err
	}
	return SyncOutput{run.ID}, s.runSync(ctx, run)
}
func (u ResumeSync) Execute(ctx context.Context, _ SyncInput) (SyncOutput, error) {
	s := u.Service
	var run domain.SyncRun
	err := s.UOW.Do(ctx, func(r Repositories) error {
		running, err := r.Runs.FindByStatus("running")
		if err != nil {
			return err
		}
		if len(running) > 0 {
			return errors.New("sync already running")
		}
		runs, err := r.Runs.FindByStatus("incomplete")
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			return errors.New("no incomplete sync to resume")
		}
		run = runs[0]
		run.Status = "running"
		run.Error = ""
		run.FinishedAt = time.Time{}
		return r.Runs.Save(run)
	})
	if err != nil {
		return SyncOutput{}, err
	}
	return SyncOutput{run.ID}, s.runSync(ctx, run)
}
func (s *SyncService) incomplete(ctx context.Context, run domain.SyncRun, cause error) error {
	run.MarkIncomplete(s.Clock.Now(), cause.Error())
	err := s.UOW.Do(context.WithoutCancel(ctx), func(r Repositories) error { return r.Runs.Save(run) })
	return errors.Join(cause, err)
}
func (s *SyncService) runSync(ctx context.Context, run domain.SyncRun) error {
	for {
		if err := ctx.Err(); err != nil {
			return s.incomplete(ctx, run, err)
		}
		batch, err := s.Supplier.FetchBatch(ctx, run.Cursor)
		if err != nil {
			return s.incomplete(ctx, run, err)
		}
		next := run
		next.Issues = append([]domain.Issue(nil), run.Issues...)
		// Finish an accepted batch even after cancellation; products and cursor commit together.
		err = s.UOW.Do(context.WithoutCancel(ctx), func(r Repositories) error {
			issues := []domain.Issue{}
			now := s.Clock.Now()
			for _, record := range batch.Records {
				if err := record.Validate(); err != nil {
					issues = append(issues, domain.Issue{BatchID: batch.ID, SKU: record.SKU, Reason: err.Error()})
					continue
				}
				old, err := r.Products.Get(record.SKU)
				if err != nil && !errors.Is(err, ErrNotFound) {
					return err
				}
				product, outcome := domain.Reconcile(old, record, batch.ID, now)
				if outcome != domain.Stale {
					if err := r.Products.Save(product); err != nil {
						return fmt.Errorf("save %s: %w", record.SKU, err)
					}
				}
			}
			next.Advance(batch.NextCursor, issues)
			if batch.NextCursor == "" {
				next.Complete(now)
			}
			return r.Runs.Save(next)
		})
		if err != nil {
			return s.incomplete(ctx, run, err)
		}
		run = next
		if run.Status == "completed" {
			return nil
		}
	}
}
