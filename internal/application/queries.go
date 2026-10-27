package application

import (
	"context"
	"fmt"
)

type GetSyncStatus struct {
	UOW   UnitOfWork
	Clock Clock
}

func (u GetSyncStatus) Execute(ctx context.Context, in StatusInput) (StatusOutput, error) {
	var out StatusOutput
	err := u.UOW.Do(ctx, func(r Repositories) error {
		run, err := r.Runs.Latest()
		if in.RunID != 0 {
			run, err = r.Runs.Get(in.RunID)
		}
		if err != nil {
			return err
		}
		to := run.FinishedAt
		if to.IsZero() {
			to = u.Clock.Now()
		}
		counts, err := r.Products.SummarizeRun(run.StartedAt, to)
		if err != nil {
			return err
		}
		out = StatusOutput{RunID: run.ID, Status: run.Status, Cursor: run.Cursor, Error: run.Error, StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, Counts: counts, InvalidCount: run.InvalidCount}
		for _, issue := range run.Issues {
			out.Issues = append(out.Issues, IssueOutput{issue.BatchID, issue.SKU, issue.Reason})
		}
		return nil
	})
	return out, err
}

type ListProducts struct{ UOW UnitOfWork }

func (u ListProducts) Execute(ctx context.Context, in ProductsInput) ([]ProductOutput, error) {
	if in.Status != "" && in.Status != "active" && in.Status != "withdrawn" {
		return nil, fmt.Errorf("invalid status %q", in.Status)
	}
	out := []ProductOutput{}
	err := u.UOW.Do(ctx, func(r Repositories) error {
		ps, err := r.Products.List(in.Status)
		if err != nil {
			return err
		}
		for _, p := range ps {
			out = append(out, ProductOutput{SKU: p.SKU, Name: p.Name, Status: p.Status, PriceCents: p.PriceCents, Version: p.Version, BatchID: p.BatchID, PreviousBatchID: p.PreviousBatchID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt})
		}
		return nil
	})
	return out, err
}
