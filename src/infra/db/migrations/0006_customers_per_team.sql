-- +goose Up
BEGIN;

-- Customers per team: when > 0, a LEAN round's customer_count is set at start
-- to customers_per_team × the teams staffed then. 0 keeps the fixed
-- customer_count (the old behaviour), so rows written before this migration
-- behave as before until they are configured or reset (the default is 18).
ALTER TABLE rounds
  ADD COLUMN customers_per_team INT NOT NULL DEFAULT 0 CHECK (customers_per_team >= 0);

COMMIT;

-- +goose Down
BEGIN;

ALTER TABLE rounds
  DROP COLUMN customers_per_team;

COMMIT;
