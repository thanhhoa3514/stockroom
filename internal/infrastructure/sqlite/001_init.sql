CREATE TABLE IF NOT EXISTS products (
 sku TEXT PRIMARY KEY,
 name TEXT NOT NULL DEFAULT '',
 price_cents INTEGER NOT NULL DEFAULT 0,
 status TEXT NOT NULL CHECK (status IN ('active','withdrawn')),
 version INTEGER NOT NULL,
 batch_id TEXT NOT NULL,
 previous_batch_id TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sync_runs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 status TEXT NOT NULL CHECK (status IN ('running','completed','incomplete')),
 cursor TEXT NOT NULL DEFAULT '',
 started_at TEXT NOT NULL,
 finished_at TEXT,
 error TEXT NOT NULL DEFAULT '',
 invalid_count INTEGER NOT NULL DEFAULT 0,
 issues TEXT NOT NULL DEFAULT '[]'
);
CREATE UNIQUE INDEX IF NOT EXISTS one_running_sync ON sync_runs(status) WHERE status = 'running';
