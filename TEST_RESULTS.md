# Stockroom Test Results

Generated: 2026-10-27T07:54:08+00:00
Toolchain: `go version go1.26.2 linux/amd64`

## Result

**PASS** — command exit code: `0`.
- Passing test entries: **41** (includes named subtests; parent tests are also counted).
- Failing test entries: **0**.
- Packages with passing tests: **6**.

Command executed against the current source:

```sh
make test TEST_FLAGS='-v -count=1'
```

`make test` enforces clean gofmt output, runs `go vet ./...`, and then runs the tests. `-count=1` disables Go test-result caching. Tests use in-memory ports or temporary SQLite databases; they do not inspect the owner’s Docker volume or production database.

## Package Results

| Package | Result | Duration |
| --- | --- | --- |
| `github.com/thanhhoa3514/stockroom/internal/application` | PASS | 0.006s |
| `github.com/thanhhoa3514/stockroom/internal/archtest` | PASS | 0.006s |
| `github.com/thanhhoa3514/stockroom/internal/domain` | PASS | 0.007s |
| `github.com/thanhhoa3514/stockroom/internal/infrastructure/fakesupplier` | PASS | 0.009s |
| `github.com/thanhhoa3514/stockroom/internal/infrastructure/sqlite` | PASS | 0.721s |
| `github.com/thanhhoa3514/stockroom/internal/interfaces/cli` | PASS | 0.003s |
| `github.com/thanhhoa3514/stockroom/cmd/stockroom` | No test files | — |

## Behavior Covered

- Domain: create, update, withdrawal, unchanged active data, equal/lower versions, equal-version withdrawal, tombstones, re-listing, validation, run completion, and issue limits.
- Application: full sync, repeated imports, supplier errors, resume from the committed cursor, refusal to start an unfinished run, cancellation, recovery, invalid records, zero changes for version-only updates, ordered records for one SKU, absent products, and the 100-issue cap across resume.
- SQLite integration: initial schema and PRAGMAs, upsert/tombstone storage, whole-batch rollback including cursor, uniqueness of running runs, reopen/resume after simulated process death following batch 2, legacy created_at migration, immutable creation time, subsecond run summaries, and issue serialization.
- Architecture: package import boundaries, standard-library-only domain, and direct time.Now restrictions in domain/application.
- Fake supplier and CLI: embedded scenario loading, outage recovery, invalid cursor handling, cancellation, command flags, and help parsing.

## Docker Verification Status

- The owner supplied a successful log for the previous `make docker-test` implementation, which ran formatting, vet, and tests inside a Docker build step. That log did not expose individual test results.
- The current implementation builds the `test` stage as `stockroom-test`, then executes a disposable container with `make test TEST_FLAGS=-v -count=1`. This is intended to stream individual results and run tests on every invocation.
- The current Docker build/run has **not** been verified in the agent environment because Docker access is restricted. The PASS result above comes from running the same test command locally, not from the updated Docker container.
- Owner command to verify the updated Docker workflow: `make docker-test`.

## Review Context for Claude

- This is a Go CLI, not an HTTP service. SQLite runs in the application process; there is no SQLite server container.
- `docker-compose.yml` persists `/data` in a named volume. Commands without `--db` use the default database; commands with `--db /data/test-outage.db` operate on a different file.
- The revised specification is `STOCKROOM_PLAN (1).md`. Newer active data with identical content advances only Version; batch metadata and UpdatedAt must remain unchanged.
- Product CreatedAt is set on initial creation (including tombstones) and preserved thereafter. Legacy rows are backfilled from UpdatedAt because the original creation time was not stored.
- Scenario identity is not persisted; resume must use the original scenario. Startup recovery assumes a leftover running process is dead, so only one updating process is supported.
- Run counts are derived from current product rows and time windows, not immutable history. Passing tests do not eliminate these documented limitations.
- `cmd/stockroom` has no automated tests. CLI tests cover argument parsing; the full Docker command workflow is not an automated end-to-end acceptance suite.

## Full Captured Output

```text
go vet ./...
go test -v -count=1 ./...
?   	github.com/thanhhoa3514/stockroom/cmd/stockroom	[no test files]
=== RUN   TestFullSyncAndRepeat
--- PASS: TestFullSyncAndRepeat (0.00s)
=== RUN   TestFailureAndResume
--- PASS: TestFailureAndResume (0.00s)
=== RUN   TestInitialFailureKeepsCatalog
--- PASS: TestInitialFailureKeepsCatalog (0.00s)
=== RUN   TestCancellationFinishesBatch
--- PASS: TestCancellationFinishesBatch (0.00s)
=== RUN   TestRecover
--- PASS: TestRecover (0.00s)
=== RUN   TestInvalidRecords
--- PASS: TestInvalidRecords (0.00s)
=== RUN   TestVersionOnlyRunHasNoChanges
--- PASS: TestVersionOnlyRunHasNoChanges (0.00s)
=== RUN   TestSameSKURecordsInOrderAndAbsentProduct
--- PASS: TestSameSKURecordsInOrderAndAbsentProduct (0.00s)
=== RUN   TestIssuesCapAcrossResume
--- PASS: TestIssuesCapAcrossResume (0.00s)
=== RUN   TestRunningRunIsNotReportedComplete
--- PASS: TestRunningRunIsNotReportedComplete (0.00s)
PASS
ok  	github.com/thanhhoa3514/stockroom/internal/application	0.006s
=== RUN   TestImports
--- PASS: TestImports (0.00s)
PASS
ok  	github.com/thanhhoa3514/stockroom/internal/archtest	0.006s
=== RUN   TestReconcile
=== RUN   TestReconcile/create
=== RUN   TestReconcile/update
=== RUN   TestReconcile/withdraw
=== RUN   TestReconcile/unchanged
=== RUN   TestReconcile/equal
=== RUN   TestReconcile/equal_withdrawal
=== RUN   TestReconcile/lower
=== RUN   TestReconcile/tombstone
=== RUN   TestReconcile/blocks_revival
=== RUN   TestReconcile/relist
--- PASS: TestReconcile (0.00s)
    --- PASS: TestReconcile/create (0.00s)
    --- PASS: TestReconcile/update (0.00s)
    --- PASS: TestReconcile/withdraw (0.00s)
    --- PASS: TestReconcile/unchanged (0.00s)
    --- PASS: TestReconcile/equal (0.00s)
    --- PASS: TestReconcile/equal_withdrawal (0.00s)
    --- PASS: TestReconcile/lower (0.00s)
    --- PASS: TestReconcile/tombstone (0.00s)
    --- PASS: TestReconcile/blocks_revival (0.00s)
    --- PASS: TestReconcile/relist (0.00s)
=== RUN   TestValidate
--- PASS: TestValidate (0.00s)
=== RUN   TestRun
--- PASS: TestRun (0.00s)
PASS
ok  	github.com/thanhhoa3514/stockroom/internal/domain	0.007s
=== RUN   TestScenarios
=== RUN   TestScenarios/basic
=== RUN   TestScenarios/repeat
=== RUN   TestScenarios/late-old
=== RUN   TestScenarios/bad-data
=== RUN   TestScenarios/outage
--- PASS: TestScenarios (0.00s)
    --- PASS: TestScenarios/basic (0.00s)
    --- PASS: TestScenarios/repeat (0.00s)
    --- PASS: TestScenarios/late-old (0.00s)
    --- PASS: TestScenarios/bad-data (0.00s)
    --- PASS: TestScenarios/outage (0.00s)
=== RUN   TestOutageRecovers
--- PASS: TestOutageRecovers (0.00s)
=== RUN   TestBadInput
--- PASS: TestBadInput (0.00s)
PASS
ok  	github.com/thanhhoa3514/stockroom/internal/infrastructure/fakesupplier	0.009s
=== RUN   TestMigrationAndUpsert
--- PASS: TestMigrationAndUpsert (0.07s)
=== RUN   TestRollbackWholeBatch
--- PASS: TestRollbackWholeBatch (0.08s)
=== RUN   TestUniqueRunningRun
--- PASS: TestUniqueRunningRun (0.06s)
=== RUN   TestCrashAfterBatchTwo
--- PASS: TestCrashAfterBatchTwo (0.13s)
=== RUN   TestUpgradeLegacyDatabase
--- PASS: TestUpgradeLegacyDatabase (0.16s)
=== RUN   TestCreatedAtPersistsAcrossChanges
--- PASS: TestCreatedAtPersistsAcrossChanges (0.07s)
=== RUN   TestRunSummarySubsecondWindowsAndVersionOnlyChange
--- PASS: TestRunSummarySubsecondWindowsAndVersionOnlyChange (0.07s)
=== RUN   TestRunIssuesRoundTrip
--- PASS: TestRunIssuesRoundTrip (0.07s)
PASS
ok  	github.com/thanhhoa3514/stockroom/internal/infrastructure/sqlite	0.721s
=== RUN   TestParse
--- PASS: TestParse (0.00s)
PASS
ok  	github.com/thanhhoa3514/stockroom/internal/interfaces/cli	0.003s
All checks passed.
```
