-- +goose Up
BEGIN;

-- =========================
-- batches: raw text submission
-- =========================
-- raw_text is the Joke Maker's unsplit blob and is live state: it is nulled
-- once Marketing splits the batch, so `raw_text IS NOT NULL` is the
-- "not yet split" predicate. raw_text_original is the immutable copy taken at
-- submit time, so an unsplit can restore the blob verbatim instead of
-- re-joining the split texts and destroying the original formatting.
--
-- No new batch_status value: "unsplit" is a derived sub-state of SUBMITTED
-- (raw_text IS NOT NULL AND no joke rows), which leaves the partial index
-- idx_batches_marketing_queue and ClaimNextBatch untouched.
ALTER TABLE batches
  ADD COLUMN raw_text          TEXT NULL,
  ADD COLUMN raw_text_original TEXT NULL;

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE batches
  DROP COLUMN raw_text,
  DROP COLUMN raw_text_original;

COMMIT;
