-- +goose Up
BEGIN;

-- =========================
-- ENUM types
-- =========================
CREATE TYPE user_role AS ENUM ('INSTRUCTOR', 'JM', 'MARKETING');
CREATE TYPE round_status AS ENUM ('CONFIGURED', 'ACTIVE', 'ENDED');
CREATE TYPE batch_status AS ENUM ('DRAFT', 'SUBMITTED', 'PROCESSED');
CREATE TYPE participant_status AS ENUM ('WAITING', 'ASSIGNED');
CREATE TYPE joke_publish_status AS ENUM ('PENDING', 'PUBLISHED', 'DISCARDED');
CREATE TYPE classification_status AS ENUM ('PENDING', 'PROCESSING', 'DONE', 'FAILED');
CREATE TYPE joke_dimension AS ENUM (
  'LENGTH',
  'TOPIC',
  'HUMOR_STYLE',
  'COMPLEXITY',
  'EDGINESS',
  'STRUCTURE',
  'WORDPLAY',
  'FRESHNESS',
  'SETUP_PAYOFF',
  'CLARITY',
  'ENERGY',
  'TITLE_FIT'
);

-- =========================
-- teams
-- =========================
CREATE TABLE teams (
  id         BIGSERIAL PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- =========================
-- users
-- =========================
CREATE TABLE users (
  user_id      BIGSERIAL PRIMARY KEY,
  display_name TEXT NOT NULL,
  role         user_role NULL,
  team_id      BIGINT NULL REFERENCES teams(id) ON DELETE SET NULL,
  status       participant_status NOT NULL DEFAULT 'WAITING',
  assigned_at  TIMESTAMPTZ NULL,
  joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_team_id_role_chk CHECK (
    (role IS NULL AND team_id IS NULL)
    OR (role IN ('JM', 'MARKETING') AND team_id IS NOT NULL)
    OR (role = 'INSTRUCTOR' AND team_id IS NULL)
  )
);

CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_team_role ON users(team_id, role);
CREATE INDEX idx_users_status ON users(status);

-- =========================
-- rounds
-- =========================
CREATE TABLE rounds (
  round_id                 BIGSERIAL PRIMARY KEY,
  round_number             INT NOT NULL,
  status                   round_status NOT NULL DEFAULT 'CONFIGURED',
  batch_size               INT NOT NULL DEFAULT 5 CHECK (batch_size >= 1),
  market_price             NUMERIC(8,2) NOT NULL DEFAULT 1.00 CHECK (market_price >= 0),
  cost_of_publishing       NUMERIC(8,2) NOT NULL DEFAULT 0.10 CHECK (cost_of_publishing >= 0),
  cost_of_discard          NUMERIC(8,2) NOT NULL DEFAULT 0.01 CHECK (cost_of_discard >= 0),
  customer_budget          NUMERIC(10,2) NOT NULL DEFAULT 3.00 CHECK (customer_budget >= 0),
  customer_count           INT NOT NULL DEFAULT 100 CHECK (customer_count >= 1),
  buy_threshold            NUMERIC(6,2) NOT NULL DEFAULT 7,
  jitter                   NUMERIC(6,2) NOT NULL DEFAULT 0.3,
  swap_margin              NUMERIC(6,2) NOT NULL DEFAULT 0.5,
  feedback_joke_count      INT NOT NULL DEFAULT 3 CHECK (feedback_joke_count >= 1),
  feedback_pass_threshold  NUMERIC(6,2) NOT NULL DEFAULT 0.75,
  is_popped_active         BOOLEAN NOT NULL DEFAULT false,
  started_at               TIMESTAMPTZ NULL,
  ended_at                 TIMESTAMPTZ NULL,
  created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_rounds_single_active ON rounds ((status)) WHERE status = 'ACTIVE';
CREATE INDEX idx_rounds_status ON rounds(status);

-- =========================
-- round_ideal_profile (excludes TITLE_FIT)
-- =========================
CREATE TABLE round_ideal_profile (
  round_id       BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  dimension      TEXT NOT NULL,
  ideal_category TEXT NOT NULL,
  PRIMARY KEY (round_id, dimension)
);

-- =========================
-- team_rounds_state
-- =========================
CREATE TABLE team_rounds_state (
  round_id          BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  team_id           BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  points_earned     INT NOT NULL DEFAULT 0 CHECK (points_earned >= 0),
  batches_created   INT NOT NULL DEFAULT 0 CHECK (batches_created >= 0),
  batches_processed INT NOT NULL DEFAULT 0 CHECK (batches_processed >= 0),
  published_jokes   INT NOT NULL DEFAULT 0 CHECK (published_jokes >= 0),
  discarded_jokes   INT NOT NULL DEFAULT 0 CHECK (discarded_jokes >= 0),
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (round_id, team_id)
);

-- =========================
-- batches
-- =========================
CREATE TABLE batches (
  batch_id     BIGSERIAL PRIMARY KEY,
  round_id     BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  team_id      BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  status       batch_status NOT NULL DEFAULT 'DRAFT',
  submitted_at TIMESTAMPTZ NULL,
  processed_at TIMESTAMPTZ NULL,
  locked_at    TIMESTAMPTZ NULL,
  locked_by    BIGINT NULL REFERENCES users(user_id) ON DELETE SET NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_batches_round_team_submitted ON batches(round_id, team_id, submitted_at);
CREATE INDEX idx_batches_marketing_queue
  ON batches (round_id, team_id, status, submitted_at)
  WHERE status = 'SUBMITTED';

-- =========================
-- jokes
-- =========================
CREATE TABLE jokes (
  joke_id        BIGSERIAL PRIMARY KEY,
  batch_id       BIGINT NOT NULL REFERENCES batches(batch_id) ON DELETE CASCADE,
  joke_text      TEXT NOT NULL,
  joke_title     TEXT NULL CHECK (char_length(joke_title) <= 120),
  publish_status joke_publish_status NOT NULL DEFAULT 'PENDING',
  published_at   TIMESTAMPTZ NULL,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_jokes_batch_id ON jokes(batch_id);

-- =========================
-- batch_submission_events (+1 submit, -1 process)
-- =========================
CREATE TABLE batch_submission_events (
  event_id    BIGSERIAL PRIMARY KEY,
  round_id    BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  team_id     BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  batch_id    BIGINT NOT NULL REFERENCES batches(batch_id) ON DELETE CASCADE,
  jokes_count INT NOT NULL CHECK (jokes_count >= 0),
  delta       INT NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_batch_submission_events_round_team_time
  ON batch_submission_events (round_id, team_id, created_at, event_id);

-- =========================
-- classification
-- =========================
CREATE TABLE classification_jobs (
  batch_id      BIGINT PRIMARY KEY REFERENCES batches(batch_id) ON DELETE CASCADE,
  round_id      BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  status        classification_status NOT NULL DEFAULT 'PENDING',
  attempts      INT NOT NULL DEFAULT 0,
  last_error    TEXT NULL,
  model         TEXT NULL,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  classified_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_classification_jobs_status ON classification_jobs (status, updated_at);

CREATE TABLE joke_dimension_values (
  joke_id   BIGINT NOT NULL REFERENCES jokes(joke_id) ON DELETE CASCADE,
  dimension joke_dimension NOT NULL,
  category  TEXT NOT NULL,
  PRIMARY KEY (joke_id, dimension)
);

CREATE TABLE joke_dim_fit (
  joke_id   BIGINT NOT NULL REFERENCES jokes(joke_id) ON DELETE CASCADE,
  dimension joke_dimension NOT NULL,
  dim_fit   NUMERIC(4,2) NOT NULL CHECK (dim_fit >= 0 AND dim_fit <= 1),
  PRIMARY KEY (joke_id, dimension)
);

CREATE TABLE joke_fit (
  joke_id     BIGINT PRIMARY KEY REFERENCES jokes(joke_id) ON DELETE CASCADE,
  round_id    BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  true_fit    NUMERIC(6,2) NOT NULL CHECK (true_fit >= 0 AND true_fit <= 12),
  computed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_joke_fit_round ON joke_fit (round_id);

-- =========================
-- AI customers + purchases
-- =========================
CREATE TABLE ai_customers (
  ai_customer_id     BIGSERIAL PRIMARY KEY,
  round_id           BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  personal_threshold NUMERIC(6,2) NOT NULL,
  starting_budget    NUMERIC(10,2) NOT NULL CHECK (starting_budget >= 0),
  remaining_budget   NUMERIC(10,2) NOT NULL CHECK (remaining_budget >= 0),
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_customers_round ON ai_customers (round_id);

CREATE TABLE purchases (
  purchase_id    BIGSERIAL PRIMARY KEY,
  round_id       BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  ai_customer_id BIGINT NOT NULL REFERENCES ai_customers(ai_customer_id) ON DELETE CASCADE,
  joke_id        BIGINT NOT NULL REFERENCES jokes(joke_id) ON DELETE CASCADE,
  team_id        BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  price          NUMERIC(8,2) NOT NULL CHECK (price >= 0),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (round_id, ai_customer_id, joke_id)
);

CREATE INDEX idx_purchases_round_joke ON purchases (round_id, joke_id);
CREATE INDEX idx_purchases_customer ON purchases (round_id, ai_customer_id);

CREATE TABLE purchase_events (
  event_id       BIGSERIAL PRIMARY KEY,
  round_id       BIGINT NOT NULL REFERENCES rounds(round_id) ON DELETE CASCADE,
  ai_customer_id BIGINT NOT NULL REFERENCES ai_customers(ai_customer_id) ON DELETE CASCADE,
  joke_id        BIGINT NOT NULL REFERENCES jokes(joke_id) ON DELETE CASCADE,
  team_id        BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  delta          SMALLINT NOT NULL CHECK (delta IN (-1, 1)),
  price          NUMERIC(8,2) NOT NULL CHECK (price >= 0),
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_purchase_events_round ON purchase_events (round_id, created_at);
CREATE INDEX idx_purchase_events_round_team_created
  ON purchase_events (round_id, team_id, created_at, event_id);

COMMIT;

-- +goose Down
BEGIN;

DROP TABLE IF EXISTS purchase_events;
DROP TABLE IF EXISTS purchases;
DROP TABLE IF EXISTS ai_customers;
DROP TABLE IF EXISTS joke_fit;
DROP TABLE IF EXISTS joke_dim_fit;
DROP TABLE IF EXISTS joke_dimension_values;
DROP TABLE IF EXISTS classification_jobs;
DROP TABLE IF EXISTS batch_submission_events;
DROP TABLE IF EXISTS jokes;
DROP TABLE IF EXISTS batches;
DROP TABLE IF EXISTS team_rounds_state;
DROP TABLE IF EXISTS round_ideal_profile;
DROP TABLE IF EXISTS rounds;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS teams;

DROP TYPE IF EXISTS joke_dimension;
DROP TYPE IF EXISTS classification_status;
DROP TYPE IF EXISTS joke_publish_status;
DROP TYPE IF EXISTS participant_status;
DROP TYPE IF EXISTS batch_status;
DROP TYPE IF EXISTS round_status;
DROP TYPE IF EXISTS user_role;

COMMIT;
