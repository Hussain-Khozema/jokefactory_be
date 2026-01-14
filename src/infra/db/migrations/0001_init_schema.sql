-- +goose Up
BEGIN;

-- =========================
-- ENUM types
-- =========================
CREATE TYPE user_role AS ENUM ('INSTRUCTOR', 'JM', 'QC', 'CUSTOMER');
CREATE TYPE round_status AS ENUM ('CONFIGURED', 'ACTIVE', 'ENDED');
CREATE TYPE batch_status AS ENUM ('DRAFT', 'SUBMITTED', 'RATED');
CREATE TYPE participant_status AS ENUM ('WAITING', 'ASSIGNED'); -- v2 waiting room
CREATE TYPE qc_tag AS ENUM (
  'EXCELLENT_STANDOUT',
  'GENUINELY_FUNNY',
  'MADE_ME_SMILE',
  'ORIGINAL_IDEA',
  'POLITE_SMILE',
  'DIDNT_LAND',
  'NOT_ACCEPTABLE',
  'OTHER'
);

-- =========================
-- teams
-- =========================
CREATE TABLE IF NOT EXISTS teams (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT NOT NULL UNIQUE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================
-- users (v2)
-- - display_name (instead of username)
-- - role/team_id nullable to support waiting/unassigned state
-- =========================
CREATE TABLE IF NOT EXISTS users (
  user_id      BIGSERIAL PRIMARY KEY,
  display_name TEXT NOT NULL,
  role         user_role NULL,
  team_id      BIGINT NULL REFERENCES teams(id) ON DELETE SET NULL,
  status       participant_status NOT NULL DEFAULT 'WAITING',
  assigned_at  TIMESTAMPTZ NULL,
  joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Role/team invariant:
-- - NULL role => team_id must be NULL (unassigned)
-- - JM/QC => team_id must be NOT NULL
-- - INSTRUCTOR/CUSTOMER => team_id must be NULL
ALTER TABLE users
  ADD CONSTRAINT users_team_id_role_chk
  CHECK (
    (role IS NULL AND team_id IS NULL)
    OR
    (role IN ('JM','QC') AND team_id IS NOT NULL)
    OR
    (role IN ('INSTRUCTOR','CUSTOMER') AND team_id IS NULL)
  );

-- =========================
-- rounds
-- =========================
CREATE TABLE IF NOT EXISTS rounds (
  round_id               BIGSERIAL PRIMARY KEY,
  round_number           INT NOT NULL,
  status                 round_status NOT NULL DEFAULT 'CONFIGURED',
  customer_budget        INT NOT NULL DEFAULT 10 CHECK (customer_budget >= 0),
  batch_size             INT NOT NULL DEFAULT 5 CHECK (batch_size >= 1),
  market_price           NUMERIC(8,2) NOT NULL DEFAULT 1 CHECK (market_price >= 0),
  cost_of_publishing     NUMERIC(8,2) NOT NULL DEFAULT 0.1 CHECK (cost_of_publishing >= 0),
  is_popped_active       BOOLEAN NOT NULL DEFAULT false,
  started_at             TIMESTAMPTZ NULL,
  ended_at               TIMESTAMPTZ NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Optional: only one ACTIVE round at a time
CREATE UNIQUE INDEX IF NOT EXISTS idx_rounds_single_active
ON rounds ((status))
WHERE status = 'ACTIVE';

-- =========================
-- round_participants (v2 waiting room)
-- =========================
-- =========================
-- team_rounds_state
-- =========================
CREATE TABLE IF NOT EXISTS team_rounds_state (
  round_id        BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  team_id         BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  points_earned   INT NOT NULL DEFAULT 0 CHECK (points_earned >= 0),
  batches_created INT NOT NULL DEFAULT 0 CHECK (batches_created >= 0),
  batches_rated   INT NOT NULL DEFAULT 0 CHECK (batches_rated >= 0),
  accepted_jokes  INT NOT NULL DEFAULT 0 CHECK (accepted_jokes >= 0),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, team_id)
);

-- =========================
-- batches
-- v2 keeps: round_id/team_id/status/submitted_at/avg_score/passes_count
-- We retain rated_at + locked_at because v2 API returns rated_at and you already use locked_at conceptually.
-- Removed qc_user_id to match v2 table list.
-- =========================
CREATE TABLE IF NOT EXISTS batches (
  batch_id      BIGSERIAL PRIMARY KEY,
  round_id      BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  team_id       BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  status        batch_status NOT NULL DEFAULT 'DRAFT',
  submitted_at  TIMESTAMPTZ NULL,
  rated_at      TIMESTAMPTZ NULL,
  avg_score     NUMERIC(4,2) NULL,
  passes_count  INT NULL CHECK (passes_count >= 0),
  feedback      TEXT NULL CHECK (char_length(feedback) <= 200),
  locked_at     TIMESTAMPTZ NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================
-- jokes
-- =========================
CREATE TABLE IF NOT EXISTS jokes (
  joke_id     BIGSERIAL PRIMARY KEY,
  batch_id    BIGINT NOT NULL REFERENCES batches(batch_id) ON DELETE CASCADE,
  joke_text   TEXT NOT NULL,
  joke_title  TEXT NULL CHECK (char_length(joke_title) <= 120),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================
-- joke_ratings
-- =========================
CREATE TABLE IF NOT EXISTS joke_ratings (
  joke_id     BIGINT PRIMARY KEY REFERENCES jokes(joke_id) ON DELETE CASCADE,
  qc_user_id  BIGINT NOT NULL REFERENCES users(user_id) ON DELETE RESTRICT,
  rating      INT NOT NULL CHECK (rating >= 1 AND rating <= 5),
  tag         qc_tag NULL,
  rated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================
-- published_jokes
-- =========================
CREATE TABLE IF NOT EXISTS published_jokes (
  joke_id     BIGINT PRIMARY KEY REFERENCES jokes(joke_id) ON DELETE CASCADE,
  round_id    BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  team_id     BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================
-- customer_round_budget
-- =========================
CREATE TABLE IF NOT EXISTS customer_round_budget (
  round_id         BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  customer_user_id BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  starting_budget  NUMERIC(10,2) NOT NULL CHECK (starting_budget >= 0),
  remaining_budget NUMERIC(10,2) NOT NULL CHECK (remaining_budget >= 0),
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, customer_user_id)
);

-- =========================
-- purchases
-- =========================
CREATE TABLE IF NOT EXISTS purchases (
  purchase_id       BIGSERIAL PRIMARY KEY,
  round_id          BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  customer_user_id  BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  joke_id           BIGINT NOT NULL REFERENCES published_jokes(joke_id) ON DELETE CASCADE,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (round_id, customer_user_id, joke_id)
);

-- =========================
-- purchase_events (audit log of buys/returns; keeps stats even after returns)
-- =========================
CREATE TABLE IF NOT EXISTS purchase_events (
  event_id          BIGSERIAL PRIMARY KEY,
  round_id          BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  customer_user_id  BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
  joke_id           BIGINT NOT NULL REFERENCES published_jokes(joke_id) ON DELETE CASCADE,
  team_id           BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  delta             SMALLINT NOT NULL CHECK (delta IN (-1, 1)),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;

-- +goose Down
BEGIN;

DROP TABLE IF EXISTS purchases;
DROP TABLE IF EXISTS purchase_events;
DROP TABLE IF EXISTS customer_round_budget;
DROP TABLE IF EXISTS published_jokes;
DROP TABLE IF EXISTS joke_ratings;
DROP TABLE IF EXISTS jokes;
DROP TABLE IF EXISTS batches;
DROP TABLE IF EXISTS team_rounds_state;
DROP TABLE IF EXISTS rounds;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS teams;

DROP TYPE IF EXISTS participant_status;
DROP TYPE IF EXISTS batch_status;
DROP TYPE IF EXISTS round_status;
DROP TYPE IF EXISTS user_role;
DROP TYPE IF EXISTS qc_tag;

COMMIT;
