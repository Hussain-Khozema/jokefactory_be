-- +goose Up
BEGIN;

-- Track which QC locked a batch to enforce assignment.
ALTER TABLE batches
  ADD COLUMN IF NOT EXISTS locked_by_qc BIGINT NULL REFERENCES users(user_id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_batches_locked_by_qc ON batches(locked_by_qc);

COMMIT;

-- +goose Down
BEGIN;

DROP INDEX IF EXISTS idx_batches_locked_by_qc;
ALTER TABLE batches DROP COLUMN IF EXISTS locked_by_qc;

COMMIT;

