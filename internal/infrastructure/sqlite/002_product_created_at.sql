ALTER TABLE products ADD COLUMN created_at TEXT NOT NULL DEFAULT '';
-- The original creation time was not stored; use the last known update for legacy rows.
UPDATE products SET created_at = updated_at;
