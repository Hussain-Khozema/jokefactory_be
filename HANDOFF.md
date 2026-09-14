# Handoff — deploy these changes to Azure

**For: Hussain.** Frank's frontend is now wired to this backend and works end to end locally.
This document is everything you need to get the same code running on Azure.

**What you need to do, in short:**

1. Take the 8 commits described below.
2. **Run the new database migration** — this is the one step that is not optional and not
   automatic.
3. Deploy the image.
4. Run the smoke checks at the bottom.

There is one new migration, two new routes, and three changed Go signatures. Nothing was
deleted, and the existing submit path still works.

---

## 1. Why these changes exist

The V2 refactor dropped several capabilities that `Backend Change Requests (V2).md` listed as
**"Unchanged"** — i.e. things the finished frontend calls. Without them the frontend cannot
complete a round. These commits restore them:

| Capability | Spec reference | What was wrong |
|---|---|---|
| `raw_text` submission | checklist line 555, `(raw_text or jokes)` | `BatchSubmitRequest` required `jokes`; a raw blob was rejected 400 |
| `split` / `unsplit` | checklist lines 564-565 | Routes did not exist |
| per-joke `sold_count` | line 557 / Item #4 | No column; the field was a Go zero value, **always 0** |
| per-joke `first_sold_at` | Item #4 | Never returned |
| `jokes_created` / `jokes_published` | Item #1 | Never returned; the frontend's waste tile read `0 − 0` |

Plus one behavioural change Frank asked for: the "at least one published joke" rule is now
**Round 1 only** (see §5).

**The workflow this restores:** the Joke Maker pastes an unsplit blob from an AI tool;
Marketing splits it into individual jokes, titles them, and publishes or discards each. That
is the intended product design — Marketing does the splitting, not the Joke Maker.

---

## 2. The commits

```
efbc0ae  feat(batches): add raw_text columns
b7cf2bc  feat(batches): accept a raw_text submission
e7f8558  feat(marketing): restore the split endpoint
40e3409  feat(marketing): restore the unsplit endpoint
aca8188  feat(marketing): scope the >=1-published rule to round 1
8f72655  fix(batches): report real per-joke sales
edf7255  feat(summary): add jokes_created and jokes_published
3f1c3ed  chore: verify phase 3B
```

`make verify` (fmt + lint + tests) passes on this set. Usecase coverage is 61.7%.

---

## 3. THE MIGRATION — do not skip this

**New file:** `src/infra/db/migrations/0002_batch_raw_text.sql`

```sql
ALTER TABLE batches
  ADD COLUMN raw_text          TEXT NULL,
  ADD COLUMN raw_text_original TEXT NULL;
```

Both columns are nullable and have no default, so **this is safe on a database with existing
rows** — old batches simply have NULL in both, which the code reads as "already split".

### This repo's first incremental migration

Until now the schema lived entirely in `0001_schema.sql`, and the README describes it that
way. That convention has to change: rewriting `0001` would break any database that has already
run it. Please update the README's Migrations section when you take this.

### Running it on Azure

```bash
DB_DSN="postgres://<user>:<password>@<host>.postgres.database.azure.com:5432/jokefactory?sslmode=require" \
  make migrate-up

DB_DSN="..." make migrate-status   # confirm 0002 shows Applied
```

Note `sslmode=require` — Azure Postgres rejects `disable`.

### ⚠️ Warning about `make migrate-down`

`migrate-down` rolls back **one** migration. That is safe while `0002` is the last applied
one. But if the database is ever in a state where `0001` is last-applied, `migrate-down`
**drops every table and type** — `0001`'s Down is a full teardown.

Please do not run `migrate-down` on Azure. If you need to undo `0002`, run the two
`DROP COLUMN` statements by hand.

### Ordering with the deploy

Deploy in this order:

1. **Migration first**, then the image. The new columns are additive and the *old* code ignores
   them, so there is no window where the running app breaks.
2. Doing it the other way round means the new code queries `raw_text` against a table that does
   not have it, and every batch query fails until the migration lands.

---

## 4. Deploy

The README says pushing to `main` triggers `.github/workflows/deploy.yml`. **That file does not
exist in this repo** — only `verify.yml` does. So deployment is manual:

```bash
az login
make deploy          # az acr build + az containerapp update; runs make verify first
```

`SKIP_VERIFY=1 make deploy` skips the verify step if you have already run it.

Environment variables are unchanged — no new ones. Keep the Container App pinned at
`min=max=1`; the dispatcher and reconciler are still in-memory and a second replica would
process classification jobs twice.

---

## 5. Behaviour changes to be aware of

**The `≥1 published joke` rule is now Round 1 only.** Previously `PublishBatch` rejected an
all-discard batch unconditionally. Frank's Round 2 exercise requires Marketing to be able to
pass on a batch entirely — the Joke Maker may submit a single weak joke, and deciding not to
publish it is the lesson. Round 1 is unchanged and still returns
`400 VALIDATION_ERROR / NO_JOKE_PUBLISHED / field: jokes`.

Mechanically: the check moved out of the repository and into a boolean the usecase computes
from `round.RoundNumber`. The repository no longer encodes a teaching rule.

**The R1 exact-batch-size and R2 cap checks moved from `Submit` to `Split`.** They are not
weakened. On the raw-text path the joke count does not exist at submit time, so the rule is
enforced at the first point it does — inside `MarketingService.Split`, returning the identical
error string (`"expected 5 jokes"`). **If you go looking for this rule in `batch.go`, it is not
there any more.**

**Three Go signatures changed**, so any code of yours that calls them needs updating:

```go
BatchService.Submit(ctx, userID, roundID, teamID int64, jokes []string, rawText string)
ports.BatchRepository.CreateBatch(ctx, roundID, teamID int64, jokes []string, rawText string)
ports.MarketingRepository.PublishBatch(..., requireAtLeastOnePublished bool)
```

`ports.MarketingRepository` also gains `SplitBatch` and `UnsplitBatch`. The in-memory test
store (`core/usecase/testutil/memstore.go`) mirrors all of it.

**New error codes the frontend had not seen before:** `BATCH_JOKES_ALREADY_DECIDED` (split or
unsplit attempted on a batch whose jokes are already published/discarded) and
`BATCH_ALREADY_PROCESSED`.

---

## 6. The two new routes

```
POST /v1/marketing/batches/:batch_id/split     { "jokes": ["...", "..."] }
POST /v1/marketing/batches/:batch_id/unsplit   (no body)
```

Both return the **same envelope as `queue/next`** — `{batch, jokes, queue_size}` — so the
frontend can drop the response straight into its queue state.

Both require the caller to **hold the batch lock**, because splitting is an edit and an expired
lock must not let a second marketer overwrite the first's work. Both refresh `locked_at`:
reading and cutting a long blob can easily exceed the 15-minute expiry.

`queue/next` now also emits `raw_text` on the batch object. The two states are mutually
exclusive: **unsplit** is `raw_text` non-null with `jokes: []`; **split** is `raw_text: null`
with the joke rows populated.

`unsplit` restores `raw_text_original`, not a re-join of the split texts — so "Back to
splitting" returns exactly what the Joke Maker pasted, with its original formatting. Verified
byte-identical locally.

---

## 7. Smoke checks after deploying

```bash
BASE=https://<your-app>.azurecontainerapps.io

curl -s $BASE/health/detailed     # expect {"status":"ok","components":{"database":{"status":"healthy"}}}
```

Then, with an instructor session and an ACTIVE round, walk the loop:

1. **Raw submit** — `POST /v1/rounds/{r}/batches` with `{"team_id":N,"raw_text":"1) ... 2) ..."}`
   → 200, `jokes_count: 0`. *(Before these changes: 400 "invalid payload".)*
2. **Claim** — `GET /v1/marketing/queue/next?round_id={r}` → `raw_text` present, `jokes: []`.
3. **Split** — `POST /v1/marketing/batches/{b}/split` with 5 jokes → `raw_text: null`, 5 joke
   ids returned.
4. **Publish** — `POST /v1/marketing/batches/{b}/publish` with one decision per joke →
   `PROCESSED`.
5. **Check the listing** — `GET /v1/rounds/{r}/teams/{t}/batches` → each joke carries
   `sold_count`, `first_sold_at`, `published_at`, `publish_status`.
6. **Check the summary** — `GET /v1/rounds/{r}/teams/{t}/summary` → `jokes_created` and
   `jokes_published` present, and `jokes_created == published_jokes + discarded_jokes`.

The single highest-value check is **step 5**: `sold_count` from that endpoint must match what
`GET /v1/rounds/{r}/market` reports for the same joke. Before these changes they disagreed —
the batches listing was structurally always 0, because there is no `sold_count` column and the
query selected seven columns, so the handler emitted a Go zero value. They now aggregate the
same `purchases` table.

(`sold_count` counts `purchases` — current holdings. `first_sold_at` comes from
`purchase_events WHERE delta = 1` — the append-only log. A joke sold and then returned by
every buyer correctly shows `sold_count: 0` with a non-null `first_sold_at`; that is the swap
behaviour, not a bug.)

---

## 8. What is NOT in this change set

So you are not surprised by what the frontend still cannot do:

- **The instructor chart series.** `GET /v1/instructor/rounds/{id}/stats` still returns
  `{round_id, leaderboard}` only. The frontend declares seven time-series arrays
  (`cumulative_sales`, `learning_curve`, `unrated_jokes_over_time`, …) and the V2 checklist
  says they "already feed the instructor charts" — they do not exist. Deferred deliberately;
  Frank is running the leaderboard alone for now.
  - Worth knowing: `ports.SalesPoint` is already declared with exactly the right json tags and
    **nothing constructs it**. `cumulative_sales` is a running `SUM(delta)` over
    `purchase_events` — probably the cheapest of the seven to add.
  - `learning_curve` and the leaderboard's `Avg Score` / `Accepted Jokes` columns are **not
    restorable** — V2 removed ratings as a concept, so per-batch "quality" no longer has a
    definition. They would need redefining against `joke_fit.true_fit`, or dropping.
- **`GET /v1/rounds/{id}/teams/{id}/kpis`** (Item #11's `KpiSnapshot`) — never built; the
  frontend derives its tiles from `/summary` + `/batches`.
- **Dedicated `ideal-profile` endpoints** (Item #5) — the frontend sets the profile through
  `POST /config`, which works.
- **Topic is not persisted.** Marketing picks a Topic in the UI but it is deliberately
  decorative — the classifier derives `TOPIC` from the joke text, and that is the value that
  counts. No column needed.

## 9. Two smaller things you may want to fix

- **`unsold_jokes` is computed wrongly** in both the summary and leaderboard queries:
  `GREATEST(published_jokes − points_earned, 0)`, where `points_earned` counts *purchase units*
  and can reach `customer_count` per joke. One hit joke selling 40 units makes `unsold_jokes`
  read 0 even when twenty other published jokes sold nothing. It should be
  `published_jokes − COUNT(DISTINCT joke_id FROM purchases)`.
- **`performance_label` is hardcoded** to `"AVERAGE PERFORMING"` for every team
  (`stats_repo.go`). Either compute it from rank or drop the field.

Neither is urgent and neither blocks the frontend.
