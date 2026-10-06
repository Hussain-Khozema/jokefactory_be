-- +goose Up
BEGIN;

-- Lean market mode. Every column has a default that reproduces CLASSIC
-- behaviour, so existing rows and requests are unaffected.
ALTER TABLE rounds
  ADD COLUMN market_mode                   TEXT          NOT NULL DEFAULT 'CLASSIC'
    CHECK (market_mode IN ('CLASSIC', 'LEAN')),
  ADD COLUMN arrival_window_seconds        INT           NOT NULL DEFAULT 900  CHECK (arrival_window_seconds >= 1),
  ADD COLUMN setup_cost_per_batch          NUMERIC(8,2)  NOT NULL DEFAULT 0    CHECK (setup_cost_per_batch >= 0),
  ADD COLUMN carrying_cost_per_joke_minute NUMERIC(8,4)  NOT NULL DEFAULT 0    CHECK (carrying_cost_per_joke_minute >= 0),
  ADD COLUMN wip_limit                     INT           NOT NULL DEFAULT 0    CHECK (wip_limit >= 0),
  ADD COLUMN preference_base               NUMERIC(6,2)  NOT NULL DEFAULT 2    CHECK (preference_base > 1);

-- Lean customers arrive over the round and shop once. NULL in CLASSIC.
ALTER TABLE ai_customers
  ADD COLUMN arrives_at TIMESTAMPTZ NULL,
  ADD COLUMN shopped_at TIMESTAMPTZ NULL;

CREATE INDEX idx_ai_customers_due
  ON ai_customers (round_id, arrives_at)
  WHERE shopped_at IS NULL AND arrives_at IS NOT NULL;

COMMIT;

-- +goose Down
BEGIN;
DROP INDEX IF EXISTS idx_ai_customers_due;
ALTER TABLE ai_customers DROP COLUMN arrives_at, DROP COLUMN shopped_at;
ALTER TABLE rounds
  DROP COLUMN market_mode, DROP COLUMN arrival_window_seconds, DROP COLUMN setup_cost_per_batch,
  DROP COLUMN carrying_cost_per_joke_minute, DROP COLUMN wip_limit, DROP COLUMN preference_base;
COMMIT;
