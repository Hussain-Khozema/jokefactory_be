-- +goose Up
BEGIN;

-- Lean fees, one rule set for both rounds. Shipping is charged per sent batch
-- by its size (small 1-4 jokes, standard 5, large 6+); storage per joke-minute
-- a batch waits on Marketing's desk; hosting per joke-minute a released joke
-- sits unsold. Every column defaults to 0, so rows written before this
-- migration carry no fees until they are configured or reset. The old
-- setup_cost_per_batch and carrying_cost_per_joke_minute columns stay, unread.
ALTER TABLE rounds
  ADD COLUMN shipping_fee_small          NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (shipping_fee_small >= 0),
  ADD COLUMN shipping_fee_standard       NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (shipping_fee_standard >= 0),
  ADD COLUMN shipping_fee_large          NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (shipping_fee_large >= 0),
  ADD COLUMN storage_fee_per_joke_minute NUMERIC(10,4) NOT NULL DEFAULT 0 CHECK (storage_fee_per_joke_minute >= 0),
  ADD COLUMN hosting_fee_per_joke_minute NUMERIC(10,4) NOT NULL DEFAULT 0 CHECK (hosting_fee_per_joke_minute >= 0);

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE rounds
  DROP COLUMN shipping_fee_small,
  DROP COLUMN shipping_fee_standard,
  DROP COLUMN shipping_fee_large,
  DROP COLUMN storage_fee_per_joke_minute,
  DROP COLUMN hosting_fee_per_joke_minute;

COMMIT;
