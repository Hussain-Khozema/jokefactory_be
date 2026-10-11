-- +goose Up
BEGIN;

-- Marketing's note to its Joke Maker. Written once, in the same transaction
-- that releases the batch or discards it as a whole, and never edited after.
-- NULL when Marketing wrote nothing, and on every batch processed before this
-- column existed, so existing rows need no backfill.
ALTER TABLE batches ADD COLUMN marketing_note TEXT NULL;

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE batches DROP COLUMN marketing_note;

COMMIT;
