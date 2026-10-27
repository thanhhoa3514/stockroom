# Stockroom: build plan (instructions for the coding agent)

Build a Go command-line application named **stockroom**. A shop keeps a local copy of its supplier's product catalog. Staff start an update, see whether it completed, see what changed, and see which problems still need attention. Supplier data is fictional (a fake supplier is part of the project).

Module path: `github.com/thanhhoa3514/stockroom`. Go 1.22+.

## 0. How to work

- The owner will be asked to explain this code and change it live **without AI**. Write plain, small, readable Go. No clever abstractions, no generics tricks, no frameworks.
- Allowed dependencies: standard library, `modernc.org/sqlite` (pure Go, no cgo). Ask before adding anything else. No ORM, no cobra (use `flag` with manual subcommands).
- Work in the steps of section 9. After each step run `gofmt -l .`, `go vet ./...`, `go test ./...` and fix everything before moving on.
- Comment only the non-obvious *why* (version rule, transaction boundary, tombstone). Do not narrate the obvious.

## 1. Architecture (DDD, strict layering)

```
cmd/stockroom/main.go                  composition root (the only place that wires layers)
internal/
  domain/                              entities, value objects, rules. Pure.
  application/                         use cases + ports (interfaces)
  infrastructure/
    sqlite/                            implements application ports (storage, unit of work)
    fakesupplier/                      implements the Supplier port with fictional data
  interfaces/
    cli/                               command parsing and output
  archtest/                            test that enforces the import rules below
```

### Import rules (hard constraints, enforced by `archtest`)

| Package | May import (inside this module) | Must NOT import |
|---|---|---|
| `domain` | nothing (standard library only) | everything else, `database/sql`, `encoding/json`, `os`, `time.Now()` calls |
| `application` | `domain` | `infrastructure/*`, `interfaces/*` |
| `infrastructure/sqlite` | `application`, `domain` | other `infrastructure/*`, `interfaces/*` |
| `infrastructure/fakesupplier` | `application`, `domain` | other `infrastructure/*`, `interfaces/*` |
| `interfaces/cli` | `application` | `domain`, `infrastructure/*` |
| `cmd/stockroom` | all of the above | n/a |

- Dependencies point inward only. No cycles.
- `interfaces/cli` talks to `application` through its input/output structs (DTOs) only, never through domain types.
- The current time comes from a `Clock` port, never from `time.Now()` inside domain or application.
- `archtest/imports_test.go`: parse every `.go` file with `go/parser` (imports only), check each package against the table, and fail with a clear message naming the offending file and import.

## 2. Domain (`internal/domain`)

Types:

- `Product`: `SKU`, `Name`, `PriceCents`, `Status` (`active` | `withdrawn`), `Version int64`, `BatchID`, `PreviousBatchID` (empty if never updated), `UpdatedAt`.
- `Record`: one incoming line from the supplier (`SKU`, `Name`, `PriceCents`, `Withdrawn bool`, `Version int64`). Has `Validate()`.
- `Batch`: `ID`, `Records []Record`, `NextCursor string` (empty means the supplier has no more data).
- `SyncRun`: `ID`, `Status` (`running` | `completed` | `incomplete`), `Cursor`, `StartedAt`, `FinishedAt`, `Error`, `Issues []Issue`, `InvalidCount`.
  Methods: `Advance(nextCursor, issues)`, `Complete(now)`, `MarkIncomplete(now, reason)`. `Complete` is only reachable when the supplier returned an empty `NextCursor`.
- `Issue`: `BatchID`, `SKU`, `Reason`.

Rule function (the heart of the project), a pure function:

```go
func Reconcile(existing *Product, in Record, batchID string, now time.Time) (Product, Outcome)
```

Outcomes: `Created`, `Updated`, `Withdrawn`, `Unchanged`, `Stale`. Invalid records are rejected earlier by `Validate()`.

Rules:

1. No existing product: create it from the record (a withdrawn record for an unknown SKU creates a **tombstone**: status `withdrawn`, empty name, price 0). This is what stops a late, older "active" record from bringing a withdrawn product back.
2. Existing product and `in.Version <= existing.Version`: **Stale**. Change nothing. This makes importing the same update twice safe, and stops older data from replacing newer data.
3. Existing product and `in.Version > existing.Version`: apply. Set `PreviousBatchID = existing.BatchID`, `BatchID = batchID`, `UpdatedAt = now`, `Version = in.Version`.
   - Withdrawn record: set status `withdrawn`; keep the old name and price (the record may not carry them).
   - Active record: update name and price, set status `active` (this also allows a newer re-listing of a withdrawn product; document it as an assumption).
   - If an active record changes no field, outcome is `Unchanged`: store only the new `Version`. Do **not** touch `BatchID`, `PreviousBatchID` or `UpdatedAt`, otherwise the run summary (section 3) would wrongly count it as an update.
4. A SKU absent from an update is **not** withdrawn. Only an explicit withdrawn record withdraws.
5. Validation (`Record.Validate`): SKU not empty, `Version > 0`, and for non-withdrawn records name not empty and `PriceCents >= 0`.

Several records for the same SKU inside one batch are applied in order; the version rule handles them.

## 3. Database (SQLite, exactly two tables)

Migration `001_init.sql`, embedded with `go:embed` and applied at startup.

```sql
CREATE TABLE products (
  sku               TEXT PRIMARY KEY,
  name              TEXT    NOT NULL DEFAULT '',
  price_cents       INTEGER NOT NULL DEFAULT 0,
  status            TEXT    NOT NULL CHECK (status IN ('active','withdrawn')),
  version           INTEGER NOT NULL,
  batch_id          TEXT    NOT NULL,
  previous_batch_id TEXT    NOT NULL DEFAULT '',
  updated_at        TEXT    NOT NULL            -- RFC3339Nano UTC (second precision is too coarse for run windows)
);

CREATE TABLE sync_runs (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  status        TEXT    NOT NULL CHECK (status IN ('running','completed','incomplete')),
  cursor        TEXT    NOT NULL DEFAULT '',    -- supplier token of the NEXT batch to fetch; '' at start
  started_at    TEXT    NOT NULL,
  finished_at   TEXT,
  error         TEXT    NOT NULL DEFAULT '',
  invalid_count INTEGER NOT NULL DEFAULT 0,
  issues        TEXT    NOT NULL DEFAULT '[]'   -- JSON array of {batch_id, sku, reason}, capped at 100
);

CREATE UNIQUE INDEX one_running_sync ON sync_runs(status) WHERE status = 'running';
```

Connection setup: `PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON`.

How "what changed" is derived (no extra table): for a run, take products with `updated_at` between the run's `started_at` and its `finished_at` (or now if still running). Created = `previous_batch_id = ''`; withdrawn = `status = 'withdrawn'`; updated = the rest. Known limit (put in README): only the latest change per product is visible, and a product created and changed again in the same run counts as updated.

The version rule compares the supplier's `version`, never `updated_at` (which is local time).

## 4. Application (`internal/application`)

Ports (interfaces defined here, implemented in infrastructure):

```go
type Supplier interface {
    FetchBatch(ctx context.Context, cursor string) (domain.Batch, error)
}
type Clock interface{ Now() time.Time }
type UnitOfWork interface {
    Do(ctx context.Context, fn func(Repositories) error) error   // one DB transaction
}
type Repositories struct {
    Products ProductRepository
    Runs     SyncRunRepository
}
```

`ProductRepository`: `Get(sku)`, `Save(product)`, `List(filter)`, `SummarizeRun(from, to)` (counts for created/updated/withdrawn). `SyncRunRepository`: `Create`, `Get`, `Latest`, `Save`, `FindByStatus`.

Use cases (each a small struct with a method, input/output DTOs in `dto.go`):

- `StartSync`: refuse if a `running` or `incomplete` run exists (tell the caller to resume). Otherwise create a run and call the shared loop.
- `ResumeSync`: load the latest `incomplete` run, set it `running`, call the shared loop from its stored `Cursor`.
- `GetSyncStatus`: latest run (or by id): status, cursor, error, counts (via `SummarizeRun`), issues. A run that is not `completed` must never be shown as complete.
- `ListProducts`: filter by status.

Shared loop `runSync(ctx, run)`:

```
loop:
  batch, err := supplier.FetchBatch(ctx, run.Cursor)      // outside any transaction
  if err: mark run incomplete (own transaction, store err), return err   // catalog untouched
  uow.Do(tx):
     for each record: Validate -> collect Issue and continue, else Reconcile and Save
     run.Advance(batch.NextCursor, issues)                 // cursor moves in the SAME transaction
     if batch.NextCursor == "": run.Complete(now)
     save run
  if run completed: return
```

- The batch's product writes and the cursor update are **one transaction**: after a crash, either the whole batch and the new cursor are stored, or neither.
- On startup (before start/resume), any run left `running` by a dead process is set to `incomplete`.
- Respect `ctx` cancellation (Ctrl-C): finish the current batch, then mark the run `incomplete` and return.
- A failure in the supplier or an interruption never deletes or overwrites existing products.

## 5. Fake supplier (`infrastructure/fakesupplier`)

Deterministic, in-memory, loaded from embedded JSON files in `scenarios/`. Selected by name. Cursor is the batch index as a string. Scenarios:

| Name | Purpose |
|---|---|
| `basic` | new products, changes, one withdrawal across 3 batches |
| `repeat` | same records delivered twice (duplicate batches) |
| `late-old` | an older version arrives after a newer one; an old "active" arrives after a withdrawal |
| `bad-data` | some invalid records (empty SKU, negative price, version 0) |
| `outage` | the supplier fails on batch N (configurable), then succeeds on the next run, to test resume |

## 6. CLI (`interfaces/cli`)

```
stockroom [--db PATH] start    [--scenario NAME]
stockroom [--db PATH] resume   [--scenario NAME]
stockroom [--db PATH] status   [--run ID]
stockroom [--db PATH] products [--status active|withdrawn]
```

Default `--db` is `./stockroom.db`. `status` prints: run id, state, last completed cursor, started/finished times, counts (created, updated, withdrawn, invalid), the error if any, and the issue list. A non-completed run prints a clear "INCOMPLETE: resume with `stockroom resume`" line. Exit code is non-zero on failure.

`cmd/stockroom/main.go` builds the SQLite database, the fake supplier, the clock, the use cases, and passes them to the CLI. Handle SIGINT/SIGTERM by cancelling the context.

## 7. Tests (required)

- `domain`: table-driven tests for `Reconcile` (create, update, withdraw, unchanged, stale equal version, stale lower version, tombstone blocks an older active record, newer active re-lists a withdrawn product, absent SKU untouched) and `Record.Validate`.
- `application`: use-case tests with in-memory fakes of the ports (`fakes_test.go`): full update completes; supplier error leaves the run incomplete and the catalog unchanged; resume continues from the cursor and completes; importing the same scenario twice creates no duplicates and the second run reports no creations; a partially processed run is never reported complete; start is refused while an incomplete run exists.
- `infrastructure/sqlite`: integration tests on a temp-file database: migrations apply, upsert and the tombstone, the transaction rolls back a whole batch when one write fails, the unique index blocks two running runs, and **a simulated crash** (close the database after batch 2, reopen, resume) loses no completed work.
- `archtest`: the import rules in section 1.

## 8. README (required content)

How to build, run, and test (`make run`, `make test`); the CLI examples above; a demo walkthrough of the `outage` scenario (start, see INCOMPLETE, resume, see COMPLETED) and of `repeat` (run twice, no duplicates); the schema and the version/tombstone rules in a few lines; **Assumptions** (absent SKU is not withdrawn, a newer active record may re-list a withdrawn product, invalid records are recorded as issues and skipped (only the first 100 are listed, the rest are counted in `invalid_count`), a withdrawal must carry a higher version than the last active record to take effect (equal version is treated as a duplicate), one update at a time and one process at a time (a `running` run found at startup is assumed dead), only the latest change per product is visible); **Unfinished work** (honest list); a short **AI usage** section left for the owner to fill in.

## 9. Order of work

1. Skeleton: go.mod, directory layout, Makefile, `archtest` (must pass on the empty skeleton).
2. `domain` with tests.
3. `application` ports, use cases, in-memory fakes, tests.
4. `infrastructure/sqlite`: migration, repositories, unit of work, integration tests.
5. `infrastructure/fakesupplier` with the scenarios.
6. `interfaces/cli` and `cmd/stockroom` wiring.
7. README.

Definition of done: `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass, `archtest` passes, and the demo walkthrough in the README works as written.
