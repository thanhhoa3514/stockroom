package application

import "time"

type Counts struct{ Created, Updated, Withdrawn int }
type SyncInput struct{}
type SyncOutput struct{ RunID int64 }
type StatusInput struct{ RunID int64 }
type StatusOutput struct {
	RunID                 int64
	Status, Cursor, Error string
	StartedAt, FinishedAt time.Time
	Counts                Counts
	InvalidCount          int
	Issues                []IssueOutput
}
type IssueOutput struct{ BatchID, SKU, Reason string }
type ProductsInput struct{ Status string }
type ProductOutput struct {
	SKU, Name, Status        string
	PriceCents, Version      int64
	BatchID, PreviousBatchID string
	CreatedAt, UpdatedAt     time.Time
}
