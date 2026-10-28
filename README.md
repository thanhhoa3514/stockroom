# Stockroom

A small Go CLI for keeping a shop's local supplier catalog up to date. All supplier data is fictional; no external service is called. It uses the standard library and the pure Go `modernc.org/sqlite` driver, without a framework or ORM.

## Build, run, test

Requires **Go 1.25+** and Make. The plan allows Go 1.22+; the pinned SQLite driver requires 1.25. Development verification used Go 1.26.2. Dependencies are pinned in `go.mod` and `go.sum`; the only direct external dependency is `modernc.org/sqlite`.

```sh
make build
./bin/stockroom --help
make run ARGS='start --scenario basic'
make run ARGS='status'
make test
```

`make test` checks formatting, runs `go vet ./...`, and runs all tests, including the architecture checks and temp-file SQLite integration tests. Build/run disable VCS stamping so they also work in source copies with unavailable Git metadata.

The default database is `./stockroom.db`. Put the global `--db` option before the command:

```sh
./bin/stockroom --db ./shop.db start --scenario basic
./bin/stockroom --db ./shop.db resume --scenario basic
./bin/stockroom --db ./shop.db status
./bin/stockroom --db ./shop.db status --run 1
./bin/stockroom --db ./shop.db products
./bin/stockroom --db ./shop.db products --status active
./bin/stockroom --db ./shop.db products --status withdrawn
```

`resume` needs an incomplete run. A fresh `start` refuses to run while an unfinished run exists. Failures return a non-zero exit code. Inspecting an incomplete run with `status` succeeds and clearly prints `INCOMPLETE`; that does not mean the update completed. Prices are integer cents. `products` also displays `CREATED_AT` and `UPDATED_AT` in UTC.

## Docker

With Make, Docker, and the Compose plugin installed, no local Go installation is needed:

```sh
make docker-build
make docker-run ARGS='start --scenario basic'
make docker-run ARGS='status'
make docker-run ARGS='products'
make docker-test
```

This is a CLI: each command starts a container and exits when done. SQLite runs inside the CLI process. Compose bind-mounts the host's `./data` directory at `/data`: `/data/stockroom.db` inside the container is `./data/stockroom.db` in this repository. The database and SQLite sidecar files survive container removal. Run one update at a time against the same database.

`make docker-run` creates `./data` if necessary and passes your host user/group IDs as `STOCKROOM_UID` and `STOCKROOM_GID`, so the container can write files you own. The runtime image remains a static binary with an unprivileged default user; Compose overrides its UID/GID for the bind mount. Direct Compose commands default to UID/GID 1000; set those variables to your own IDs when different. Database files are ignored by Git, and `data` is excluded from the Docker build context.

`make docker-build` builds the runtime image. `make docker-run` forwards `ARGS` to the CLI through Compose. `make docker-test` builds a separate test image, then runs formatting checks, vet, and all tests in a disposable container without touching the runtime data volume. Each invocation runs tests again with `-v -count=1`, printing individual test names and PASS/FAIL results directly to the terminal; success ends with `All checks passed.` The existing `make build`, `make run`, and `make test` commands use Go on your host.

To try the outage walkthrough with a separate database in the same directory:

```sh
make docker-run ARGS='--db /data/outage-demo.db start --scenario outage'
# Expected exit 1; the first batch is preserved.
make docker-run ARGS='--db /data/outage-demo.db status'
make docker-run ARGS='--db /data/outage-demo.db resume --scenario outage'
make docker-run ARGS='--db /data/outage-demo.db status'
```

`docker compose down`, including with `--volumes`, does not delete the host's `./data` directory. Databases from the previous named-volume configuration remain in that old Docker volume; they are not automatically copied into `./data`. Switching mounts starts with separate data unless you explicitly copy the old database and any SQLite sidecar files while no updater is running. Do not remove the old volume until you have verified any needed data was transferred.

## Outage demo

`outage` contains **6 batches and 121 incoming records**: 40 initial products, price changes, withdrawals, repeated and late records, re-listings, version-only updates, one unknown-SKU tombstone, and 3 invalid records.

For a fresh `./data/demo.db`, run:

```sh
make build
mkdir -p data
./bin/stockroom --db ./data/demo.db start --scenario outage --fail-batch 4
# Expected exit 1: batches 1–3 are committed, with 40 products.
./bin/stockroom --db ./data/demo.db status
# INCOMPLETE; Created: 30; Updated: 10; next supplier cursor is cur_... .
./bin/stockroom --db ./data/demo.db resume --scenario outage
./bin/stockroom --db ./data/demo.db status
# COMPLETED; Created: 25; Updated: 12; Withdrawn: 4; Invalid: 3.
./bin/stockroom --db ./data/demo.db products --status withdrawn
./bin/stockroom --db ./data/demo.db start --scenario outage --fail-batch 0
./bin/stockroom --db ./data/demo.db status
# COMPLETED; Created: 0; Updated: 0; Withdrawn: 0; Invalid: 3.
```

The final catalog has 41 SKUs: 37 active and 4 withdrawn. Reimporting records still reports the 3 invalid inputs while making no product changes. Counts summarize the latest state, so they can change between the partial run and completion. The recording walkthrough is in `DEMO_SCRIPT.md`.

On `start`, `--fail-batch N` chooses the one-based failing batch (default 2; 0 disables failure). The fake fails once per supplier instance. On `resume`, the composition root disables the simulated outage to represent supplier recovery in a new CLI process. This is a demo mechanism, not a real network retry policy. Always resume with the **same scenario** used to start.

## Repeat demo

```sh
./bin/stockroom --db ./repeat-demo.db start --scenario repeat
./bin/stockroom --db ./repeat-demo.db status
# Run 1: COMPLETED, Created: 2.
./bin/stockroom --db ./repeat-demo.db start --scenario repeat
./bin/stockroom --db ./repeat-demo.db status
# Run 2: COMPLETED, Created: 0; Updated: 0; Withdrawn: 0.
./bin/stockroom --db ./repeat-demo.db products
# Still exactly two products: APPLE and TEA, both at version 1.
```

## Supplier cursors

Embedded batches include a `next_cursor` such as `cur_a1b2...`. The fake supplier recognizes this opaque token; the application never increments or decodes it. It only stores the returned token in the same transaction as the accepted batch and sends it back on the next fetch. Tokens remain stable across supplier instances so a new process can resume. They are pagination identifiers, not security credentials.

The four unchanged small scenarios still accept their original numeric checkpoints, then return tokens for subsequent batches. The expanded `outage` fixture accepts its own tokens only. A cursor from a different snapshot or unrelated scenario is rejected. Use a fresh demo database after upgrading from the original three-batch outage fixture; existing product data is not deleted, but an incomplete run from that old fixture requires the previous application version to resume.

Other scenarios:

- `late-old`: older product data arrives after a newer version, including an older active record after a withdrawal. Neither replaces the newer data.
- `bad-data`: three invalid records are skipped and recorded as issues; the two valid products still import, and the run completes.

## Code map

```text
cmd/stockroom/main.go                 composition root, clock, signals
internal/domain/                     pure catalog rules and run lifecycle
internal/application/                ports, DTOs, use cases, shared sync loop
internal/infrastructure/sqlite/       embedded migration, repositories, transactions
internal/infrastructure/fakesupplier/ embedded JSON scenarios
internal/interfaces/cli/             flag parsing and output using application DTOs
internal/archtest/                    import rules and time.Now checks
```

Start reading at `domain.Reconcile`, then `application.SyncService.runSync`, then `sqlite.Database.Do`. The CLI only imports application DTOs and use cases. Infrastructure implements the application ports; `main` wires the concrete implementations.

The tests cover reconciliation and validation, retries and resume, cancellation, invalid data, import boundaries, transaction rollback, the running-run unique index, and reopening a database after a simulated crash following batch 2. Additional cases cover version-only updates reporting zero changes, equal-version withdrawals, ordered records for the same SKU, absent products, the 100-issue cap across resume, subsecond run windows, creation-time preservation, and upgrading legacy databases.

## Storage and consistency

The embedded `001_init.sql` is applied idempotently at startup. Existing databases without `products.created_at` are upgraded transactionally by `002_product_created_at.sql`; no extra migration table is created. Legacy rows use their existing `updated_at` as the best available creation-time estimate because the original time was not recorded. New rows record the actual local creation time, including tombstones; updates, withdrawals, re-listings, and retries preserve it. There are two application tables:

| Table | Purpose |
| --- | --- |
| `products` | One row per SKU: name, integer price, status, supplier version, current/previous batch, local creation and update times. |
| `sync_runs` | Run lifecycle, next-fetch cursor, timestamps, error, total invalid count, and JSON issues capped at 100. |

SQLite also maintains its internal `sqlite_sequence` for run IDs. A partial unique index permits only one `running` row. Each connection uses WAL, a 5000 ms busy timeout, and foreign keys enabled. Times are UTC RFC3339 with fixed nanosecond precision so text range comparisons are ordered correctly.

Supplier versions determine freshness: equal or lower versions are ignored, including duplicate batches. A newer withdrawn record keeps the existing name and price. An unknown withdrawal creates an empty tombstone, preventing older active data from reviving the SKU. A newer active record can re-list it. An unchanged active record with a newer version advances only the version; it preserves the batch metadata, `created_at`, and `updated_at`, so the run reports no product change.

A supplier fetch happens outside the transaction. All valid records in a batch, its issues, and its next cursor commit together. If a write fails, the whole batch rolls back. Supplier errors leave committed products untouched and mark the run incomplete. Resume uses its saved cursor and original run ID. `SIGINT`/`SIGTERM` cancel fetching; an already accepted batch finishes before the run becomes incomplete. If the final batch commits during cancellation, the run is completed because there is no work left.

Before start/resume, a leftover `running` row is marked incomplete for recovery. Run only one updating CLI process against a given database: this recovery assumes the previous process has stopped.

## Assumptions

- A SKU absent from an update is not withdrawn; withdrawal must be explicit.
- A newer active record may re-list a withdrawn product. A withdrawal must also have a higher version than the last active record; an equal version is a duplicate and does not withdraw it.
- Invalid records are recorded as issues and skipped. `Invalid` counts all of them, even after the 100-issue display/storage cap.
- Records for the same SKU within a batch are applied in supplier order.
- Only one update process operates at a time; the index is a storage guard, not a process lock.
- Cursors are opaque supplier tokens; an empty cursor means the initial fetch. Completion is only set after receiving a terminal batch. Numeric checkpoints are accepted only for compatibility with the four unchanged small scenarios.
- Change counts are derived from current product rows whose local update times fall within the run. Only the latest change per product is visible. A product created and changed again in the same run counts as updated (or withdrawn if its final state is withdrawn). Newer identical active records do not count as changes because only their version advances. Later runs can make earlier run summaries lose rows; this is not an audit log.
- Resume uses the same scenario as start. Scenario identity is not persisted in the two-table schema.

## Unfinished work

- No real supplier adapter, network retry/backoff, or authentication; only deterministic embedded scenarios.
- No cross-process ownership lock or automatic distinction between a crashed process and a still-running process.
- No immutable change history; historical counts have the limits described above.
- No persisted scenario identity or general schema version history; startup currently checks the schema and applies the creation-time upgrade when needed.

## AI usage

### Tools used

For larger projects where correctness and maintainability matter, I generally prefer models from Anthropic and OpenAI. For this assessment, I used OpenAI Codex as a coding agent and Claude as a design advisor.

### Solution design

I discussed the assessment's data model and edge cases with Claude, including version-based conflict handling, tombstones for withdrawn products, and storing the supplier cursor in the same transaction as the batch. I proposed the solution based on my experience and made the final decisions about the design and scope.

The submitted implementation stores one current row per SKU and updates it in place when a newer supplier version is accepted. It does not implement append-only product history; the limitations of this choice are documented above.

### Code generation

Codex scaffolded and implemented the project from a written plan that I reviewed and adapted. Its contributions included the layered structure, application ports, reconciliation logic, SQLite adapter and migrations, CLI, automated tests, Docker configuration, and documentation. I also used it to refine the fictional supplier scenarios and prepare the demo walkthrough.

### My contribution

I defined and refined the requirements and assumptions, reviewed the proposed approach, and ran the application scenarios and tests. I used the results to question and refine the behavior, including batch recovery, withdrawal handling, version ordering, timestamps, and the local development workflow. I remain responsible for the submitted solution and its documented tradeoffs.

