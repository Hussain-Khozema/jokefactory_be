# Joke Factory API

Backend for the Joke Factory classroom simulation. Students write jokes (JM),
Marketing titles and publishes them, an LLM classifies published jokes, and
simulated AI customers buy/swap based on fit to a hidden ideal profile.

## Architecture

Hexagonal (ports & adapters) layout:

```
src/
  app/                 # HTTP layer
    server/            # router, DI, lifecycle
    http/{handler,dto,response}/
    middleware/
  core/                # business logic (no I/O)
    domain/            # entities, enums, errors
      scoring/         # pure 3-tier fit + length classifier
    ports/             # repository, classifier, dispatcher interfaces
    usecase/           # session, instructor, batch, marketing,
                       # classification, aicustomer, feedback, stats
  infra/
    config/ logger/
    db/                # postgres pool + migrations + WithTx
    repo/postgres/     # one file per aggregate
    llm/               # Azure Foundry classifier (+ stub)
    worker/            # in-memory dispatcher + reconciler
```

- **Core** depends on nothing external.
- **Infra** implements ports.
- **App** wires HTTP → usecases → ports.

## Game flow

1. Students join; instructor assigns JM + Marketing per team and configures the round (including hidden `ideal_profile`).
2. Instructor starts the round → AI customers generated with personal buy thresholds.
3. JM submits batches; Marketing claims, titles, publishes/discards.
4. Published batches are classified asynchronously (Length in code + LLM for 11 dims).
5. Fit is materialized; AI customers buy/swap against `true_fit`.
6. Teams poll feedback (curated Good/Improve dims) and summary; instructor polls leaderboard stats.

## API (base `/v1`)

Identity: student routes use `X-User-Id`. Instructor routes require instructor auth.

| Method | Path | Notes |
|--------|------|-------|
| POST | `/session/join` | Join lobby |
| GET | `/session/me` | Current user |
| POST | `/instructor/login` | Instructor auth |
| GET | `/rounds/active` | Public round list (hides engine knobs) |
| POST | `/rounds/{id}/batches` | JM submit |
| GET | `/rounds/{id}/teams/{tid}/batches` | Team batches |
| GET | `/rounds/{id}/teams/{tid}/summary` | Team card |
| GET | `/rounds/{id}/teams/{tid}/feedback` | Curated dim feedback |
| GET | `/rounds/{id}/market` | Published jokes + sold counts |
| GET | `/marketing/queue/next` | Claim next batch |
| POST | `/marketing/batches/{id}/publish` | Publish/discard |
| GET | `/marketing/queue/count` | Queue depth |
| GET | `/instructor/rounds/{id}/lobby` | Lobby snapshot |
| POST | `/instructor/rounds/{id}/config` | Round params + ideal profile |
| POST | `/instructor/rounds/{id}/assign` | Assign teams |
| PATCH/DELETE | `/instructor/rounds/{id}/users/{uid}` | Manage users |
| POST | `/instructor/rounds/{id}/start` \| `/end` \| `/popups` | Lifecycle |
| GET | `/instructor/rounds/{id}/stats` | Leaderboard |
| POST | `/admin/reset` | Wipe game state |
| GET | `/health` \| `/health/detailed` | Health checks |

Errors: `{ "error": { "code", "message", "request_id" } }`.

## Configuration

All env vars use the `APP_` prefix.

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_PORT` | `8080` | HTTP port |
| `APP_HOST` | `0.0.0.0` | Bind host |
| `APP_LOG_LEVEL` | `info` | debug/info/warn/error |
| `APP_LOG_FORMAT` | `json` | json/text |
| `APP_ADMIN_PASSWORD` | (set in env) | Instructor login |
| `APP_DB_HOST` | `localhost` | Postgres host |
| `APP_DB_PORT` | `5432` | Postgres port |
| `APP_DB_USER` | `postgres` | DB user |
| `APP_DB_PASSWORD` | `postgres` | DB password |
| `APP_DB_NAME` | `jokefactory` | Database name |
| `APP_DB_SSLMODE` | `disable` | Use `require` on Azure |
| `APP_LLM_BASE_URL` | _(empty)_ | Foundry `/openai/v1/` endpoint |
| `APP_LLM_API_KEY` | _(empty)_ | Foundry API key |
| `APP_LLM_DEPLOYMENT` | `gpt-4o-mini` | Deployment name |
| `APP_LLM_TEMPERATURE` | `0` | Sampling temperature |
| `APP_LLM_MAX_RETRIES` | `3` | Classifier retries |

When `APP_LLM_BASE_URL` / `APP_LLM_API_KEY` are unset, the **stub classifier** is used (local/dev).

## Local development

```bash
# Dependencies
go mod download

# Postgres (sslmode=disable)
docker compose up -d   # or: make docker-up
make migrate-up

# Run API
make run
# → http://localhost:8080

# Checks
make verify   # fmt + lint + tests
```

## Migrations

Goose SQL schema lives in a single file:

`src/infra/db/migrations/0001_schema.sql`

Fresh deploy: point goose at an empty Postgres database and run:

```bash
make migrate-up
make migrate-status
```

To wipe and recreate locally: drop/recreate the DB (or `make migrate-down`), then `migrate-up` again.
## Testing

```bash
make test
make verify
```

Phase flow tests live under `src/core/usecase/` (`phase3`…`phase10_e2e`). Scoring unit tests are in `src/core/domain/scoring/`.

## Azure deployment

Target stack (see `REFACTOR_PLAN.md` Appendix C):

- **Azure Container Apps** (pin `min=max=1` — in-memory queue/workers)
- **Azure Database for PostgreSQL Flexible Server** (`APP_DB_SSLMODE=require`)
- **Azure AI Foundry** (`gpt-4o-mini`)
- **ACR** + **Key Vault** for image/secrets

Provision with the `az` CLI sketch in Appendix C, then:

1. Set env vars / Key Vault secret refs on the Container App.
2. `make migrate-up` against the Azure DSN.
3. Build/push image (`az acr build` or GitHub Actions).
4. Smoke `/health` and a full classroom flow.

> Horizontal scale later requires replacing the in-memory dispatcher with a shared broker; the reconciler covers restarts today.

## Make targets

```bash
make help          # list targets
make run build test lint fmt verify
make migrate-up migrate-down migrate-status
make docker-up docker-down
make deploy        # az acr build + update Container App (needs az login)
```

## CI / CD

- **PRs:** `.github/workflows/verify.yml` runs `make verify`.
- **Local deploy:** `make deploy` (or `SKIP_VERIFY=1 make deploy` after `az login`).
- **Push to `main`:** `.github/workflows/deploy.yml` (needs GitHub OIDC setup).

One-time GitHub Actions OIDC setup: see [`docs/GITHUB_DEPLOY.md`](docs/GITHUB_DEPLOY.md).

## License

See [LICENSE](LICENSE) if present.
