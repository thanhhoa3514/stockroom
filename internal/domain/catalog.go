package domain

import (
	"errors"
	"time"
)

type Product struct {
	SKU, Name                string
	PriceCents               int64
	Status                   string
	Version                  int64
	BatchID, PreviousBatchID string
	CreatedAt, UpdatedAt     time.Time
}
type Record struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	Withdrawn  bool   `json:"withdrawn"`
	Version    int64  `json:"version"`
}

func (r Record) Validate() error {
	if r.SKU == "" {
		return errors.New("SKU must not be empty")
	}
	if r.Version <= 0 {
		return errors.New("version must be positive")
	}
	if !r.Withdrawn && r.Name == "" {
		return errors.New("name must not be empty")
	}
	if !r.Withdrawn && r.PriceCents < 0 {
		return errors.New("price must not be negative")
	}
	return nil
}

type Batch struct {
	ID         string
	Records    []Record
	NextCursor string
}
type Outcome string

const (
	Created   Outcome = "created"
	Updated   Outcome = "updated"
	Withdrawn Outcome = "withdrawn"
	Unchanged Outcome = "unchanged"
	Stale     Outcome = "stale"
)

func Reconcile(existing *Product, in Record, batchID string, now time.Time) (Product, Outcome) {
	p := Product{SKU: in.SKU, Status: "active", CreatedAt: now}
	outcome := Created
	if existing != nil {
		p = *existing
		if in.Version <= p.Version {
			return p, Stale
		}
		if !in.Withdrawn && p.Status == "active" && p.Name == in.Name && p.PriceCents == in.PriceCents {
			p.Version = in.Version
			return p, Unchanged
		}
		p.PreviousBatchID = p.BatchID
		outcome = Updated
	}
	p.Version = in.Version
	p.BatchID = batchID
	p.UpdatedAt = now
	if in.Withdrawn {
		// An unknown withdrawal retains a tombstone so older active records cannot revive it.
		p.Status = "withdrawn"
		if existing != nil {
			outcome = Withdrawn
		}
	} else {
		p.Name = in.Name
		p.PriceCents = in.PriceCents
		p.Status = "active"
	}
	return p, outcome
}

type Issue struct {
	BatchID string `json:"batch_id"`
	SKU     string `json:"sku"`
	Reason  string `json:"reason"`
}
type SyncRun struct {
	ID                    int64
	Status, Cursor        string
	StartedAt, FinishedAt time.Time
	Error                 string
	Issues                []Issue
	InvalidCount          int
	terminal              bool
}

func (r *SyncRun) Advance(nextCursor string, issues []Issue) {
	r.Cursor = nextCursor
	r.terminal = nextCursor == ""
	r.InvalidCount += len(issues)
	for _, issue := range issues {
		if len(r.Issues) < 100 {
			r.Issues = append(r.Issues, issue)
		}
	}
}
func (r *SyncRun) Complete(now time.Time) {
	if !r.terminal {
		return
	}
	r.Status = "completed"
	r.FinishedAt = now
	r.Error = ""
}
func (r *SyncRun) MarkIncomplete(now time.Time, reason string) {
	r.Status = "incomplete"
	r.FinishedAt = now
	r.Error = reason
}
