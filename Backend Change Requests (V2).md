# Backend Change Requests — V2

> **For: Hussain (backend) + his AI assistant.** Single source of truth for V2 backend changes.
> Companion design doc: [[V2-Workflow]].
> **Frank's V2 frontend is essentially complete**, currently running on the in-browser mock API. The items below are the backend changes needed to make the real BE match the finished FE so it can reconnect.
> *Phase sequencing removed (FE is done, so we no longer need to gate BE work on FE reconnect). Build order is flexible — dependencies are noted inline. Old item numbers retained for cross-reference with prior conversations.*



## How to read

- Each item uses the same template: **Status · Why · Current state · Required change · DB · API · Acceptance.**
- "Current state" reflects the existing Backend Document — **flag any inaccuracies.**
- Keep everything scoped by `round_id`. Keep changes **additive**; call out any breaking change.



## Status legend

✅ READY · 🟡 DESIGN (open questions, review first) · ⚠️ CONFIRM (design changed since the earlier draft — Frank to verify) · ⏸️ HELD (deferred).

---



# Context — the AI Customer engine (read this first)

The single biggest change in V2 is replacing human customers with **AI customers**: deterministic, reproducible demand that reacts to how well each joke matches a hidden "ideal joke." This context frames Items #5, #9, and #10 — read it before those.

**The model (locked).**

- **100 AI customers per round** (instructor-configurable). They all share **one hidden ideal joke profile** — a chosen level on each of the **12 dimensions** (see rubric in Item #9), set by the instructor before the round starts.
- Each customer has a **budget of $3** and can hold up to **3 jokes** at once.
- **Every published joke is classified once**, at publish time, against the 12-dimension rubric — partly by rules, partly by one GPT-4o-mini call. This classification is **cached** and drives *all* downstream math. **No LLM call ever happens per customer.**

**How a joke's appeal is scored.**

- `true_fit(joke)` = sum over the 12 dimensions of `dim_fit(joke's level, ideal's level)`. **Range 0–12.** A joke that matches the ideal on every dimension scores 12; a joke that matches on none scores ~0.
- Per customer, a tiny **deterministic jitter** (±0.3, hashed from `customer_id + joke_id + session_seed`) is added → `perceived_fit`. Same customer + same joke → same perceived fit, every run.

**Buy / return behaviour (per customer, per 15s tick).**

- **Accumulate** (has budget): buy the highest-`perceived_fit` unowned jokes that clear the **buy threshold τ = 7**, until budget runs out.
- **Swap** (out of budget): if the best available joke beats the customer's weakest held joke by more than the **swap margin M = 0.5**, return the weak one and buy the better one. Otherwise hold.
- Each customer buys a given joke at most once (existing `ALREADY_BOUGHT`). A great joke can therefore sell to up to 100 customers.
- Customer re-evaluation is **staggered** across the 15s tick so sales trickle in visually instead of bursting.

**The net effect teams experience.** A perfect-fit joke sells to ~all 100 customers; a fit-7 joke sells to ~30–80 (jitter-dependent); a fit-6 joke sells to a few or none. Teams must **reverse-engineer the hidden ideal** from the feedback they're given.

**Learning-design constraint (important for Item #10).** Teams must **not** be handed a clean, full per-dimension reveal of the ideal — that makes the whole exercise trivial. All feedback to Marketing is **deliberately partial and curated** (see Item #10). **There is no class-wide feed** — each Marketing team sees only its *own* team's recent joke performance. This supersedes any earlier design with a class-wide live feed or per-sale random reveals.

All engine constants (τ, M, budget, jitter, tick, customer count) are **instructor-configurable so we can tune them during the pilot** — see the "Instructor-Configurable Parameters" section next.

---



# Instructor-Configurable Parameters (for pilot tuning)

**Why this matters.** We don't yet know the best values for the economy or the AI-customer engine — the pilot is how we find them. So **every tunable below must be settable by the instructor** (via the round config / start endpoints and the instructor UI), *not* hardcoded. Defaults are our current best guess; the "pilot range" is the span we expect to sweep.

**Current state (flag for Hussain).** Today the FE only sends a tiny subset: `POST /config` accepts `{ customer_budget, batch_size }`, and `POST /start` accepts `{ customer_budget, batch_size, market_price, cost_of_publishing }`. Everything else below is **new** and needs to be accepted + persisted per round and returned by `GET /v1/rounds/active`.


| Parameter                 | Default         | Pilot range | Set via                | Notes                                                             |
| ------------------------- | --------------- | ----------- | ---------------------- | ----------------------------------------------------------------- |
| `market_price`            | $1.00           | 0.50–2.00   | config/start           | revenue per sale                                                  |
| `cost_of_publishing`      | $0.10           | 0.05–0.25   | config/start           | Marketing cost per published joke (Item #1)                       |
| `cost_of_discard`         | $0.01           | 0.00–0.05   | config/start           | **NEW** — Marketing cost per discarded joke (Item #1)             |
| `customer_budget`         | $3.00           | 2–5         | config/start           | per-AI-customer budget (jokes each can hold)                      |
| `batch_size`              | 5               | 3–10        | config/start           | R1 fixed size / R2 cap                                            |
| `customer_count`          | 100             | 50–200      | config                 | AI customers per round (Item #5)                                  |
| `buy_threshold` (τ)       | 7 of 12         | 6–9         | config                 | min perceived fit to buy (Item #5)                                |
| `swap_margin` (M)         | 0.5             | 0.25–1.0    | config                 | how much better a joke must be to trigger a return/swap (Item #5) |
| `jitter`                  | ±0.3            | 0–0.5       | config                 | per-customer taste noise (Item #5)                                |
| `tick_seconds`            | 15              | 5–30        | config                 | engine re-evaluation interval (Item #5)                           |
| `ideal_profile`           | (12 dim levels) | any         | ideal-profile endpoint | the hidden target joke (Item #5); locked at round start           |
| `llm_call_ceiling`        | 1000/round      | 500–2000    | config                 | classifier cost guardrail (Item #9)                               |
| `feedback_joke_count`     | 3               | 1–5         | config                 | how many recent jokes Marketing feedback shows (Item #10)         |
| `feedback_pass_threshold` | 0.75            | 0.5–0.9     | config                 | dim_fit cutoff for a ✓ (Item #10)                                 |
| `session_seed`            | (auto)          | —           | config (optional)      | fixes deterministic jitter for reproducible pilots                |


**Acceptance.** Instructor can set every parameter above before a round; the value persists per round and is echoed back on `GET /v1/rounds/active` (or a dedicated config-read endpoint). The engine, classifier, and feedback all read the round's configured values — no recompilation needed to retune between pilot runs.

---



# Item 1: Cost Model — all cost borne by Marketing · ⚠️ CONFIRM (revised per instructor feedback)

**Why.** Earlier we planned a per-joke **creation** cost charged to the Joke Makers. The instructor pushed back: charging JMs per joke nudges them to *under*-produce, which is the opposite of what we want — JMs should crank out the maximum number of jokes. **Decision: move all cost onto Marketing instead.** Marketing pays for what it publishes and a smaller amount for what it throws away, so it prioritises carefully while JMs stay free to over-produce.

**Instructor's exact guidance (paraphrased).** Joke Makers create for free. Marketing incurs **$0.10 for every joke it publishes** and **$0.01 for every joke it discards** (creates but chooses not to publish). Tell all roles about this upfront, but keep it low-salience — it should appear on the **Marketing dashboard**, and **not** be prominent (or shown at all) on the **Joke Maker dashboard**.

**Current state.** Round config has `market_price` (1), `cost_of_publishing` (0.10), `unsold_jokes_penalty`. `profit` is computed server-side. (The earlier draft's `cost_of_creation` idea is **dropped** — do not implement it.)

**Required change.**

1. **No creation cost.** JMs are never charged. Do not add `cost_of_creation`.
2. Add round field `cost_of_discard` NUMERIC(6,2), default **0.01**.
3. Keep `cost_of_publishing` (default **0.10**), now conceptually a Marketing cost.
4. New profit formula (all costs are Marketing's):
  ```
   discarded_jokes = created_jokes − published_jokes        (created but never published)

   profit = (sold_jokes      × market_price)
          − (published_jokes × cost_of_publishing)   ← $0.10 each, Marketing
          − (discarded_jokes × cost_of_discard)      ← $0.01 each, Marketing
  ```
5. **Remove** `unsold_jokes_penalty` from the profit calculation (column may stay dormant). It double-punishes a failure already priced into `cost_of_publishing`.

**DB.** `rounds`: add `cost_of_discard`; keep `cost_of_publishing`; `cost_of_creation` not added; `unsold_jokes_penalty` no longer used in `profit`.

**API.**

- `GET /v1/rounds/active` → include `cost_of_publishing` and `cost_of_discard`.
- Instructor `/config` + `/start` → accept `cost_of_publishing` and `cost_of_discard`.
- `GET /…/teams/{team_id}/summary` → include `jokes_created` (int) and `jokes_published` (int) so the FE can derive discards; add `cost_breakdown { revenue, publish_cost, discard_cost, profit }`. FE cannot derive `jokes_created` (R2 has variable batch sizes).
- Instructor `/stats` leaderboard `profit` uses the new formula.

**Acceptance.**

- Sold joke → `+market_price`.
- Published-but-unsold joke → costs `cost_of_publishing` (0.10), nothing more.
- Created-but-discarded joke → costs `cost_of_discard` (0.01).
- JMs are never charged for creating jokes.
- `unsold_jokes_penalty` no longer affects `profit`.
- `jokes_created` and `jokes_published` accurate; `cost_breakdown` reconciles to `profit`.

**⚠️ FE reconciliation needed (this cost flip postdates the finished FE).** The current FE still encodes the *old* creation-cost model and must be updated alongside the BE:

- `config/simConfig.ts` → `economics` still has `costOfCreation`; replace with `costOfDiscard`.
- `services/economics.ts` → `computeProfit` still does `sold*price − created*creation − published*publish`; change to `sold*price − published*publish − discarded*discard`.
- `cost_breakdown` currently returns a `production_cost` field; rename/repurpose to `discard_cost` on both BE and FE.
- Instructor `/start` payload currently sends `cost_of_publishing` only; add `cost_of_discard` (and drop any creation cost).

---



# Item 2: Force-Release ≥1 Joke per Batch · ✅ READY

**Why.** Marketing should prioritise, not gatekeep; customers must always have stock.

**Current state.** Only rating = 5 publishes.

**Required change.** On batch rating:

- Publish all 5-rated jokes.
- If none scored 5, publish the single **highest-rated**.
- Tie with no 5 → publish Marketing's designated joke (see API tiebreak below).
- Compliance-fail batch → publish nothing.

**API.** `POST /v1/qc/batches/{batch_id}/ratings`: accept optional `tiebreak_joke_id`; if absent on a tie, server picks deterministically (lowest `joke_id`). Response `published.count ≥ 1` for any non-compliance-fail batch.

**Acceptance.** Every rated non-fail batch yields ≥1 published joke.

---



# Item 3: Topic Palette + Title + Marketing Tagging · 🟡 DESIGN

**Marketing tags each joke at rating time — JM does NOT tag.** At the moment Marketing chooses which jokes to publish, it supplies two things per joke: a **Topic** and a **Title**.

## 3a. Topic palette (list locked)

**Locked Topic categories** (professor's 10-value rubric):
Workplace · MBA / Student Life · Tech · AI · Animals · Sports · Everyday life · Social media · Education · Random / absurd.
Banned (compliance fail): religion, politics, tragedy, targeting a person/group.

Topic becomes dimension #2 in the classifier (Item #9) — this avoids an LLM call for Topic.

## 3b. Title (NEW)

**Marketing enters a Title for each joke while selecting jokes to submit/publish.** The title is scored by the AI customer engine as the **12th dimension, "Title Fit"** (Item #9): does the title thematically match the joke, or is it random/junk? This discourages Marketing from typing something arbitrary.

**Open design questions.**

1. Reuse `joke_ratings.tag` ENUM vs. a dedicated `topic` field on the rating? **Recommend a dedicated** `topic` **field** for clarity.
2. Add a `title` TEXT field on the rating/publish payload (required for every published joke; UI blocks submission without it).
3. Required on every rated joke, or only published jokes? **Recommend: Topic + Title required for every joke Marketing publishes.**

**API.** Return `topic` and `title` in market + batch listings (needed by the classifier and by Marketing feedback). Marketing-side rating payload accepts both.

**Acceptance.** Marketing UI requires Topic + Title per published joke; backend persists both and returns them on the market + batches endpoints.

---



# Item 7: QC → "Marketing" Rename · 🟡 DESIGN (likely FE-only)

**Proposed.** Keep backend role enum `QC` + `/qc/` routes unchanged; FE renames to "Marketing" in user-facing copy only. Confirm with Frank if a backend rename is preferred.

**Acceptance.** No backend change required if FE-only is approved.

---



# Item 9: Joke Dimension Classifier — 12 dimensions · 🟡 DESIGN

**Why.** The customer engine (Item #5) scores each joke against the **12-dimension rubric**. The classifier runs once at publish, caches the result, and powers all downstream math. Single biggest LLM-touching piece.

**LLM choice (locked).** **OpenAI GPT-4o-mini**, temperature 0. One batched call per joke at publish.

**Trigger.** When a joke transitions `RATED → published` (via Item #2's force-release), classify it synchronously on the request thread or via a fast worker (publish-blocking is fine at demo scale).

**Rule-classified dims (computed in-process, no LLM):**

1. **Length** — word-count buckets → Short (≤25) / Medium (26–60) / Long (≥61). 3 levels.
2. **Topic** — copy Marketing's tag from Item #3a. 10 values.
3. **Structure Format** — heuristics: Q&A if "?" in setup; List if newlines/numbered/bullets; Dialogue if quoted lines; Short story if >2 sentences and no other pattern; else One-liner if very short, else Setup-punchline. LLM fallback only on ambiguous cases (optional).

**LLM-classified dims (batched, single call per joke):**
4. **Humor Style** (8): Pun / Observational / Irony / Absurdity / Exaggeration / Self-deprecating / Anti-joke / Callback
5. **Complexity** (5): Very simple / Simple / Moderate / Thoughtful / Expert
6. **Edginess** (2): Clean / Slightly edgy
7. **Wordplay Density** (4): None / Light / Moderate / Heavy
8. **Topical Freshness** (5): Timeless / Slightly current / Current / Very topical / Time-sensitive
9. **Setup-to-Payoff Ratio** (5): Immediate / Quick / Balanced / Long / Very long build
10. **Clarity vs Ambiguity** (5): Crystal clear / Mostly clear / Slightly ambiguous / Ambiguous / Reinterpretation
11. **Energy / Delivery** (5): Deadpan / Low / Conversational / Animated / High-energy

**NEW — 12th dimension:**
12. **Title Fit** (graded, 5 levels): **Perfect / Strong / Moderate / Weak / Mismatch**. LLM rates how well Marketing's entered `title` (Item #3b) suits the joke's theme — a graded score, not a yes/no. **Note:** unlike dims 1–11 (matched against the hidden *ideal profile*), Title Fit is **intrinsic** — it scores execution quality (title-vs-joke), not taste match. It contributes a **graded value in [0, 1]** to `true_fit`, mapped from the level: Perfect 1.0 · Strong 0.75 · Moderate 0.5 · Weak 0.25 · Mismatch 0. A well-titled joke earns close to a full point; a random/junk title earns near 0.

**Prompt shape (illustrative — adapt to your code style).** Include the joke text and, for Title Fit, the entered title; ask for strict JSON with one level per LLM dimension. Use OpenAI `response_format: { type: "json_object" }` (or `json_schema` for stricter validation).

**Cache.** Store in `joke_classifications` (one row per `(joke_id, dim_id)`, **12 rows per joke**). Never re-classify the same joke.

**Cost guardrail.** Per-round LLM call counter; beyond a configured ceiling (default 1000 calls/round), log + skip classification (treat joke as unclassified — engine won't consider it).

**Error handling.** If the LLM call fails after 2 retries: log, mark joke classification-failed, do **not** block publishing. Engine treats unclassified jokes as `true_fit = 0` (they won't sell).

**DB.**

- `joke_classifications`: `joke_id` (FK), `dim_id` (ENUM/INT over the 12 dims), `level` (TEXT). PK = `(joke_id, dim_id)`. ~12 rows per published joke.
- (Optional) `joke_classification_meta`: `joke_id`, `classified_at`, `llm_model`, `llm_call_id`, `status` (ok/failed).

**API.**

- Internal — runs server-side at publish.
- Debug endpoint: `GET /v1/instructor/jokes/{joke_id}/classification` → the 12 dim levels for that joke.

**Acceptance.**

- Every published joke has 12 dim levels cached within ~2s of publish.
- Same joke text + title + GPT-4o-mini + temp 0 → same classification across runs (modulo model variance).
- Cost stays under guardrail; failed classifications don't break publishing.

---



# Item 5: AI Customer Engine · 🟡 DESIGN (model locked, operational items open)

See the **Context** section above for the full narrative. This item is the implementation spec.

**Model (locked — single shared taste + per-customer jitter).**

- **100 customers per round** (instructor-configurable). All share **one hidden ideal profile** = a chosen level on each of the **12 dimensions**, set by the instructor before the round (see *Instructor-Configurable Parameters*).
- **Budget = $3** per customer (holds up to 3 jokes).
- **Buy threshold τ = 7** (out of 12). **Swap margin M = 0.5.** **Jitter = ±0.3.** **Tick = 15s.** All configurable.

**Fit computation (per joke, once at publish — after Item #9 fires).**

```
true_fit(joke) = Σ over 12 dims of dim_fit(joke.level_d, ideal.level_d)
   - Ordinal dims: dim_fit = 1 − |joke_pos − ideal_pos|
     - Length (3 levels): positions {0, 0.5, 1}
     - Complexity, Freshness, Setup-Payoff, Clarity, Energy (5 levels): {0, 0.25, 0.5, 0.75, 1}
     - Wordplay (4 levels): {0, 0.33, 0.67, 1}
   - Categorical/Binary dims (Topic, Humor Style, Edginess): dim_fit = 1 if exact match, else 0
   - Title Fit (intrinsic, graded, not vs ideal): dim_fit = level score in {1.0, 0.75, 0.5, 0.25, 0}
     for Perfect / Strong / Moderate / Weak / Mismatch
```

**Range 0–12.** Cache as `true_fit` per joke (or compute on the fly from cached classifications + ideal).

**Per-customer perceived fit (pure math every tick — never an LLM call).**

```
jitter(c, j) = uniform[-0.3, +0.3], deterministic from hash(customer_id, joke_id, session_seed)
perceived_fit(c, j) = clamp(true_fit(j) + jitter(c, j), 0, 12)
```

**Buy / return per customer per tick.**

```
ACCUMULATE (budget ≥ 1):
   buy unowned jokes with perceived_fit ≥ τ, highest perceived_fit first, until budget runs out
SWAP / RETURN (budget < 1):
   J = best unowned (≥ τ); W = lowest perceived_fit held
   if perceived_fit(J) − perceived_fit(W) > M → return W, buy J
   else hold
```

Each customer buys a joke at most once (existing `ALREADY_BOUGHT`). A great joke can sell to up to 100 customers.

**Staggering.** Each customer's re-evaluation is offset randomly in [0, 15s] against the tick so sales trickle visually.

**DB.**

- `customers` (round-scoped): `customer_id`, `round_id`, `budget`, `created_at`. No taste columns (shared ideal).
- `round_ideal_profile`: `round_id`, `dim_id`, `ideal_level`. One row per (round, dim) — **12 rows per round**.
- Add `session_seed` (INT) to `rounds` for deterministic jitter.
- `joke_true_fits` (optional cache): `joke_id`, `round_id`, `true_fit`. Computed once after classification; recompute only if the instructor edits the ideal mid-round (locked at round start by default).

**API.**

- Internal worker tick (15s) drives the engine; use existing `/buy` and `/return` endpoints internally as the customer's actor.
- Instructor endpoints (debug/MVP):
  - `GET /v1/instructor/rounds/{round_id}/ideal-profile` — read the ideal (12 dims).
  - `POST /v1/instructor/rounds/{round_id}/ideal-profile` — set it (locked once round goes ACTIVE).
  - Engine settings (τ, M, budget, jitter, tick, customer count) read from the round's **instructor config** — settable before each round so we can tune during the pilot (see *Instructor-Configurable Parameters*).

**Operational items (open).**

- Worker loop vs cron scheduler (recommend simple in-process async loop for MVP).
- Round end: freeze customers, stop ticking, no more purchases.
- Customer init timing (create when round → ACTIVE).
- `true_fit` recompute if ideal changes (default: ideal locked at round start → no recompute).

**Acceptance.**

- After round start, customers tick every 15s with staggered phases.
- A perfect-fit (12) joke sells to ~100; a fit-7 joke to ~30–80; a fit-6 joke to few or none.
- Returns happen only when budget is spent and a new joke beats the lowest held by > 0.5.
- Same `(customer, joke)` → same perceived fit within a round.

---



# Item 10: Marketing Feedback — curated per-joke dimension checks (NEW) · ⚠️ CONFIRM

**Why.** Marketing needs an actionable signal to reverse-engineer the ideal, without a clean full reveal. This is the existing feedback panel, redesigned deliberately. **This is the only feedback mechanism** — there is no class-wide feed; each team sees only its own recent jokes.

**What the FE shows (for reference — FE is built).**

- The team's **latest N published jokes** for the Marketing user's own team. **N is instructor-configurable** (default **3**) — how many jokes' performance Marketing can see.
- Per joke, **5 dimension checkmarks** (of the 12), each pass ✓ or fail ✗, reflecting that joke's **actual** score on those dims vs the hidden ideal.
- **Selection rule (curated toward failures):** target **3 fail + 2 pass**. If fewer than 3 fail exist, backfill with passes; if fewer than 2 pass exist, backfill with fails. Always exactly 5 shown.
- A **"joke profile criteria"** list (the 12 dimensions) shown **above** the feedback section, **click-to-unfold**.

**Pass/fail definition.** A dimension "passes" for a joke if its `dim_fit` against the ideal is **≥ 0.75** (this threshold is **instructor-configurable**; exact-match categorical dims pass at 1.0; Title Fit passes at level score ≥ 0.75, i.e. Strong or Perfect).

**Learning-design rationale.** Showing only **5 of 12** dims, weighted toward failures, gives teams something to act on while keeping the full ideal hidden.

**DB.** None new — derive from cached `joke_classifications` + `round_ideal_profile`. Add config values (`feedback_joke_count` default 3, `feedback_pass_threshold` default 0.75) to the round/engine config.

**API.**

- `GET /v1/marketing/teams/{team_id}/feedback` (X-User-Id must be Marketing on that team) → latest N published jokes, each with `{ joke_id, title, topic, checks: [ { dim_id, dim_name, pass: bool } × 5 ] }`, selected per the rule above. N from round config.
- The 12-criteria definitions can be FE-hardcoded or exposed via a small `GET /v1/meta/dimensions` endpoint — Frank to decide (likely FE).

**Acceptance.**

- Marketing sees its team's latest N published jokes (N per instructor config, default 3), each with exactly 5 dim checks reflecting real pass/fail at the configured threshold.
- Selection honours the 3-fail/2-pass target with backfill.
- Full per-dimension breakdown is never exposed; only the curated 5 per joke.

---



# Item 4: Lead-Time KPI Support · 🟡 DESIGN

**Why.** Lead time (joke created → joke first sold) is the headline R1→R2 proof metric; FE can't compute it today.

**Current state.** `batch.submitted_at` (created proxy), `batch.rated_at` (published proxy), `purchase_events.created_at` (sold) — sold time not surfaced per joke.

**Required.** Expose per-joke timestamps in the team batches endpoint:

- `published_at` (when the joke was force-released / classified)
- `first_sold_at` (earliest `purchase_events.created_at` for this joke, or null)

**API.** Add these fields to each joke in `GET /v1/rounds/{round_id}/teams/{team_id}/batches`.

**Acceptance.** FE can compute lead time per joke and aggregate per team / round / round-number.

---



# Item 11: Live Team KPI Panel · 🟡 DESIGN (primary)

**Why.** Give each team an always-on scoreboard so they *feel* the effect of a better batch and improve round-over-round. **Deliberately kept to a few plain-language numbers** — students have little time to learn the app, so no jargon and no ratios to interpret.

**What the FE shows (team's own team only, updating live as sales tick):** four simple tiles —

1. **Profit** ($) — the headline scoreboard. Tap to expand a plain breakdown: revenue, publishing cost, discard cost.
2. **Jokes Sold** — running count of units sold (the satisfying live number).
3. **Made → Published → Sold** — a small funnel of three counts. Conveys "how much we made vs. kept vs. actually sold" visually, so teams grasp waste and market-fit *without* needing the words *yield* or *sell-through*.
4. **Avg. Time to First Sale** — average time from a joke being created to its first sale. This is the operational **lead time** metric in plain words; its drop from R1→R2 is the core continuous-improvement proof.

No standalone ratio metrics (yield %, sell-through %, throughput) — the funnel communicates the same idea more intuitively.

**DB.** None new — aggregate from existing joke/purchase data + Item #4 timestamps.

**API — designed for reuse and future growth.** One `KpiSnapshot` object, reused at team scope (this item) and round scope (Item #12), so the FE has a single renderer and new metrics slot in without new endpoints. The panel shows only the curated four tiles, but the payload carries the **full** metric set — the FE decides what to surface, and we can expose more later without a contract change.

- **Endpoint (round-scoped for R1↔R2 comparison, consistent with the rest of the API):**
`GET /v1/rounds/{round_id}/teams/{team_id}/kpis` (team-scoped auth; light polling, e.g. 3–5s).
- `KpiSnapshot` **shape (documented):**
  ```jsonc
  {
    "schema_version": 1,          // bump only on breaking change; new fields are additive
    "as_of": "2026-07-16T12:00:00Z", // compute time — lets the FE show freshness
    "round_id": 2,
    "round_number": 2,
    "team_id": 7,

    "funnel": {                   // distinct jokes at each stage (ints)
      "made": 40,                 // jokes created
      "published": 12,            // jokes released by Marketing
      "sold": 9                   // distinct jokes with ≥1 sale
    },
    "units_sold": 213,            // total sale events (a joke may sell to many customers)

    "profit": 189.30,             // USD, authoritative (server-computed)
    "cost_breakdown": {
      "revenue": 213.00,
      "publish_cost": 1.20,       // published × cost_of_publishing
      "discard_cost": 0.28        // (made − published) × cost_of_discard
    },

    "lead_time_seconds": {        // object, not a scalar — room to grow
      "avg": 142,                 // headline "Avg. Time to First Sale" (Item #11 tile)
      "median": 130,              // reserved for future display
      "best": 61,                 // reserved
      "latest": 158               // reserved
    },

    "ratios": {                   // carried for instructor/debrief + future FE use; NOT shown on the student tiles
      "first_pass_yield": 0.30,   // published / made
      "sell_through": 0.75        // sold / published
    }
  }
  ```
- **Field contract / documentation rules (so future devs can extend safely):**
  - `schema_version` bumps **only** on a breaking change (rename/remove/retype). Adding a new metric field is additive and does **not** bump it — clients must ignore unknown fields.
  - `profit` and `cost_breakdown` are **server-authoritative**; the FE never recomputes money.
  - `funnel.sold` (distinct jokes) vs `units_sold` (total sales) are intentionally separate — see the economics question in *Changes to discuss with Hussain*.
  - All monetary fields USD, 2-decimal; all durations in **seconds** (suffix `_seconds`); ratios are 0–1 floats.
  - Team-scoped only — no cross-team data in this response (consistent with the removed class-wide feed).
- **Future extension (noted, not built now):** `GET /v1/rounds/{round_id}/teams/{team_id}/kpis/timeline?bucket=30s` for a sparkline; a `compare` view diffing two rounds. Both reuse `KpiSnapshot` — no redesign needed.

**Acceptance.** Each team sees its own four tiles, updating within a poll cycle as sales occur; `lead_time_seconds.avg` drops visibly when a team improves. The response validates against `KpiSnapshot` v1 and carries the full metric set even though only four tiles render.

---



# Item 12: Instructor Continuous-Improvement Visualization · ⏸️ HELD (basics only — finalize at meeting)

**Why.** For the debrief: show the class that R2 beat R1 — the payoff of continuous improvement.

**Basic ideas (to refine at the meeting).**

- **Per-round aggregate snapshot per team** so the FE can graph **R1 vs R2 deltas**: lead time ↓, the made→published→sold funnel, profit ↑.
- **Stage cycle times** (create → rate → publish → first sold) to show *where* the delay was and whether the bottleneck moved between rounds.
- **Class improvement leaderboard** — who cut lead time the most R1→R2.

**API (tentative — reuses Item #11's** `KpiSnapshot`**).**
`GET /v1/instructor/rounds/{round_id}/kpis` →

```jsonc
{
  "schema_version": 1,
  "round_id": 2,
  "round_number": 2,
  "teams": [ KpiSnapshot, KpiSnapshot, ... ],   // one per team, same shape as Item #11
  "round_aggregate": KpiSnapshot,               // class totals in the same shape (team_id: null)
  "stage_cycle_times_seconds": {                // instructor-only, for bottleneck analysis
    "create_to_rate": { "avg": 90 },
    "rate_to_publish": { "avg": 20 },
    "publish_to_first_sale": { "avg": 142 }
  }
}
```

- Reusing `KpiSnapshot` means the instructor view and the team panel share one renderer and one field contract — future metrics appear in both for free.
- **R1↔R2 comparison** is a client-side diff of two round snapshots (fetch `round_id=1` and `=2`); a future `GET /v1/instructor/kpis/compare?rounds=1,2` can serve it server-side if the diff gets heavy. Noted, not built now.

**Status.** HELD — build Item #11's `KpiSnapshot` first; this endpoint is mostly a wrapper over it plus stage cycle times. Which graphs, and how much to show live vs. in debrief, to be decided with Frank at the meeting.

---



# Cross-cutting notes

- All new tables are **additive**; existing tables and endpoints are unchanged unless flagged.
- **Cost is Marketing-only** (Item #1): $0.10 per published, $0.01 per discarded, nothing on JMs. Communicate to all roles upfront but keep it off (or low-salience on) the JM dashboard.
- The rubric is **12 dimensions**, fit range **0–12**, τ = 7. Dims 1–11 are matched against the hidden ideal; the 12th (Title Fit) is **intrinsic and graded** (title-vs-joke, [0,1]).
- **No class-wide feed and no SSE.** The only Marketing feedback is per-team, curated, poll-based (Item #10: 5 of 12 dims, failure-weighted, N jokes configurable). No clean per-dimension reveal anywhere — learning-design requirement.
- **Instructor-configurable knobs:** engine constants (τ, M, budget, jitter, tick, customer count), plus `feedback_joke_count` (default 3) and `feedback_pass_threshold` (default 0.75).
- LLM hosting: **OpenAI GPT-4o-mini**, `OPENAI_API_KEY` in the BE environment, structured JSON output mode, temperature 0.
- Reproducibility: temperature 0 + deterministic jitter + locked ideal at round start = same demand structure every run; same individual sale outcomes within a session.

---



# Changes to discuss with Hussain (Frank ↔ Hussain sync)

These need a **joint** BE + FE decision — not something either side should lock unilaterally. Bring to the meeting.

1. **Revenue ceiling per joke (economics).** A single joke can sell to up to `customer_count` (default 100) AI customers. Confirm revenue = `units_sold × market_price` (so a perfect joke can earn ~$100/round), and that `profit` counts **units**, not distinct jokes. This sets the whole payoff curve — agree before tuning economy params. *(Affects Item #1, KPI* `funnel.sold` *vs* `units_sold`*.)*
2. **KPI endpoint vs FE-derive (Item #11).** Build the `KpiSnapshot` endpoint server-side, or have the FE derive KPIs from the existing `/summary` + `/batches` (lead time already computed in `economics.ts`)? Recommend the endpoint for a clean, reusable contract — but it's Hussain's build cost. Decide who owns lead-time computation.
3. **Config storage & rollout (Configurable Parameters).** How to persist the 15 tunables — discrete `rounds` columns, or a single extensible `engine_config` JSON column (easier to grow)? And which params must land for the **first pilot** vs. later. Recommend a JSON config blob for forward-compat.
4. `cost_breakdown.production_cost → discard_cost` **(Item #1).** This is a coordinated **breaking rename** — BE and FE must ship together, or the money panel breaks. Agree on the cutover.
5. **Engine execution on Render (Item #5).** Where does the 15s tick run — in-process async loop on the web service, or a separate background worker? Confirm Render's tier supports a long-running worker and how round-end stops it cleanly.
6. **Classifier timing & Title plumbing (Item #9).** Synchronous-at-publish (blocks the rating response ~1–2s) vs. async worker; and confirm the classifier receives Marketing's entered `title` so **Title Fit** (dim #12, intrinsic graded) can be scored and stored in `joke_classifications`.
7. **Forward-compat contract (**`schema_version`**).** Agree the rule now: new response fields are additive and clients ignore unknowns; `schema_version` bumps only on rename/remove/retype. This keeps BE and FE from breaking each other as V2 grows.
8. **Confirm removals.** Drop the class-wide feed, SSE, and per-sale `revealed_dim` tables/endpoints (Items #6/#8) — verify Hussain hasn't already started them.

---



# Backend contract checklist (every endpoint the finished FE calls)

Verified against the FE's mock API (`services/mockApi.ts` + service layer) — the mock is the de-facto spec of what the real BE must return. **Unchanged** = already in the old backend, keep as-is. **Changed** = existing endpoint, new fields/behavior. **New** = build from scratch.

**Session / rounds**

- `POST /v1/session/join` · Unchanged
- `GET /v1/session/me` · Unchanged
- `GET /v1/session/team` · Unchanged
- `GET /v1/teams` · Unchanged
- `GET /v1/rounds/active` · **Changed** — echo all new config params (Configurable Parameters)

**Instructor**

- `POST /v1/instructor/login` · Unchanged
- `GET /v1/instructor/rounds/{id}/lobby` · Unchanged
- `POST /v1/instructor/rounds/{id}/assign` (`customer_count`, `team_count`) · Unchanged
- `PATCH` / `DELETE /v1/instructor/rounds/{id}/users/{uid}` · Unchanged
- `POST /v1/instructor/rounds/{id}/popups` · Unchanged
- `POST /v1/instructor/rounds/{id}/start` · **Changed** — accept `cost_of_discard` + engine params
- `POST /v1/instructor/rounds/{id}/config` · **Changed** — accept the full tunable set
- `POST /v1/instructor/rounds/{id}/end` · Unchanged
- `GET /v1/instructor/rounds/{id}/stats` · **Changed** — `leaderboard.profit` uses new formula (Item #1); arrays (learning_curve, output_vs_rejection, revenue_vs_acceptance, cumulative_sales, batch_quality_by_size) already feed the instructor charts / Item #12
- `GET|POST /v1/instructor/rounds/{id}/ideal-profile` · **New** (Item #5)
- `GET /v1/instructor/jokes/{id}/classification` · **New** debug (Item #9)
- `GET /v1/instructor/rounds/{id}/kpis` · **New**, HELD (Item #12)

**Team / JM**

- `POST /v1/rounds/{id}/batches` (raw_text or jokes) · Unchanged
- `GET /v1/rounds/{id}/teams/{id}/summary` · **Changed** — `cost_breakdown` uses `discard_cost`; `jokes_created` / `jokes_published` (Item #1)
- `GET /v1/rounds/{id}/teams/{id}/batches` · **Changed** — per-joke `published_at`, `first_sold_at`, `sold_count`, `topic`, `joke_title` (Items #3, #4)
- `GET /v1/teams/{id}/kpis` · **New** (Item #11) — *or* derive FE-side from summary + batches (lead time already computed by `services/economics.ts::computeLeadTimeSeconds`)

**QC / Marketing**

- `GET /v1/qc/queue/count` · Unchanged
- `GET /v1/qc/queue/next` · Unchanged
- `POST /v1/qc/batches/{id}/split` · Unchanged
- `POST /v1/qc/batches/{id}/unsplit` · Unchanged
- `POST /v1/qc/batches/{id}/ratings` · **Changed** — accept `joke_title` + `topic` per rating (Item #3); force-release ≥1 (Item #2)
- `GET /v1/marketing/teams/{id}/feedback` · **New** (Item #10)

**Customer** (human-customer endpoints stay; the AI engine drives buy/return internally)

- `GET /v1/rounds/{id}/customers/budget` · Unchanged
- `GET /v1/rounds/{id}/market` · **Changed** — `joke_title`, `topic`/`category`, `bought_count`
- `POST /v1/rounds/{id}/market/{jid}/buy` · Unchanged (engine actor)
- `POST /v1/rounds/{id}/market/{jid}/return` · Unchanged (engine actor)

**Coverage verdict.** Every endpoint the FE calls is accounted for above. The DB additions the doc requires: `cost_of_discard` + engine/feedback config on `rounds`; `joke_classifications` (12 rows/joke); `round_ideal_profile` (12 rows/round); `customers`; `session_seed`; and per-joke `published_at` / `first_sold_at` / `topic` / `joke_title` / `title_fit` surfaced on batches. No class-wide feed / SSE / per-sale reveal tables.

---



# Changelog

- **2026-07 (rev 5)** — Upgraded the KPI APIs (Items #11/#12) into a documented, reusable `KpiSnapshot` contract (round-scoped, schema-versioned, full metric set in payload with only curated tiles shown; future timeline/compare extensions noted). Added a **Changes to discuss with Hussain** section for joint BE↔FE decisions (revenue ceiling per joke, KPI endpoint vs derive, config storage, cost-field rename cutover, engine execution on Render, classifier timing + Title plumbing, forward-compat policy, removals).
- **2026-07 (rev 4)** — Cross-checked against the finished FE's mock API. Added **Instructor-Configurable Parameters** table (all economy + engine + feedback tunables settable per round for pilot tuning — flipped from "code-config MVP"). Added **Item 1 FE-reconciliation** notes (old `costOfCreation` model still in `simConfig.ts` / `economics.ts` / `cost_breakdown.production_cost`). Added **Backend contract checklist** enumerating every FE endpoint (unchanged / changed / new) + DB-additions verdict.
- **2026-07 (rev 3)** — Added **Item 11** (live team KPI panel — 4 plain-language tiles: Profit, Jokes Sold, Made→Published→Sold funnel, Avg Time to First Sale; team-scoped, poll-based) and **Item 12** (instructor R1→R2 continuous-improvement visualization, HELD until meeting). KPI set deliberately jargon-free for fast student usability.
- **2026-07 (rev 2)** — Title Fit changed from binary to **graded [0,1]** (5 levels). **Removed the class-wide feed and SSE entirely** (Items 6 & 8) — Marketing sees only its own team's latest N jokes via Item #10, poll-based. Feedback **joke count** and **pass threshold (0.75)** made instructor-configurable.
- **2026-07** — Removed phase sequencing (FE V2 complete). Added AI-customer **Context** section. **Item 1 cost model flipped to Marketing-borne** ($0.10 publish / $0.01 discard; no JM creation cost) per instructor feedback. Added **Title** entry (Item 3b) and **Title Fit** as the **12th dimension**; rubric and fit range now **12 / 0–12**. Added **Item 10** (curated per-joke Marketing feedback, 5 of 12 dims, failure-weighted). Old item numbering retained for cross-reference.
- **2026-06** — (superseded) Phased build sequence; GPT-4o-mini, SSE for feed, Length 3-level buckets, code-config first (MVP), 11-dimension rubric with per-sale drip reveal.

