-- +goose Up
BEGIN;

-- rounds
CREATE INDEX IF NOT EXISTS idx_rounds_status ON rounds(status);

-- users (handy for roster rendering)
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_team_role ON users(team_id, role);
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);

-- batches
CREATE INDEX IF NOT EXISTS idx_batches_round_team_submitted
ON batches(round_id, team_id, submitted_at);

-- QC queue: earliest SUBMITTED batch in a round
CREATE INDEX IF NOT EXISTS idx_batches_qc_queue
ON batches(round_id, status, submitted_at)
WHERE status = 'SUBMITTED';

-- jokes
CREATE INDEX IF NOT EXISTS idx_jokes_batch_id ON jokes(batch_id);

-- published jokes
CREATE INDEX IF NOT EXISTS idx_published_jokes_round ON published_jokes(round_id);

-- purchases
CREATE INDEX IF NOT EXISTS idx_purchases_round_customer
ON purchases(round_id, customer_user_id);

-- purchase_events
CREATE INDEX IF NOT EXISTS idx_purchase_events_round_created
ON purchase_events(round_id, created_at, event_id);
CREATE INDEX IF NOT EXISTS idx_purchase_events_round_team_created
ON purchase_events(round_id, team_id, created_at, event_id);

COMMIT;

-- +goose Down
BEGIN;

DROP INDEX IF EXISTS idx_purchases_round_customer;
DROP INDEX IF EXISTS idx_purchase_events_round_team_created;
DROP INDEX IF EXISTS idx_purchase_events_round_created;
DROP INDEX IF EXISTS idx_published_jokes_round;
DROP INDEX IF EXISTS idx_jokes_batch_id;
DROP INDEX IF EXISTS idx_batches_qc_queue;
DROP INDEX IF EXISTS idx_batches_round_team_submitted;
DROP INDEX IF EXISTS idx_users_team_role;
DROP INDEX IF EXISTS idx_users_role;
DROP INDEX IF EXISTS idx_rounds_status;

COMMIT;
