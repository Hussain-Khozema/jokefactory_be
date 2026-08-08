# Joke Classifier Sandbox — Build Plan

> A self-contained spec for building a small experimentation web app. This document assumes **no prior knowledge** of the larger project. Everything you need is here.

---

## 1. Context & background

We are building a classroom "Joke Factory" simulation game. In the game, students write jokes and an audience of **AI customers** decides whether to "buy" each joke. The AI customers judge a joke on **12 dimensions** and compare it to a hidden **ideal joke profile** chosen by the instructor. The closer a joke matches the ideal profile, the higher its "fit" score.

Before we build all of this into the main system, the professor wants a **tiny standalone tool to experiment**: pick an ideal profile, paste a joke, and see (a) how an LLM classifies that joke across the dimensions and (b) how well it scores against the chosen ideal. This helps us validate the dimensions, categories, and the scoring math with real jokes.

**This app is a throwaway experimentation sandbox.** It is not connected to the main game backend or database. Keep it simple.

---

## 2. Objective

A single-page web app where an instructor can:
1. **Configure the ideal joke profile** — choose one category per dimension.
2. **Submit a joke** (text + title, both required) to be classified.
3. See the **LLM's chosen category for each dimension** (and the code-derived category for Length).
4. See a **per-dimension fit score (`dim_fit`)** and an **overall `true_fit`** computed exactly like the main system.
5. See the **raw classification JSON** returned by the model.

---

## 3. Scope

**In scope**
- One joke at a time.
- Ideal-profile configuration UI.
- LLM classification of a single joke + Length via code.
- Fit scoring (`dim_fit` per dimension + overall `true_fit`).
- Display of results + raw JSON.

**Out of scope (do NOT build)**
- Authentication / user accounts.
- Database / persistence (in-memory or browser state is fine; nothing needs to survive a refresh).
- Batches, multiple jokes, AI-customer buying, budgets, rounds, teams, or any game logic beyond classification + fit.
- Deployment/hosting concerns (local dev is enough).

---

## 4. The 12 dimensions (full specification)

Each joke is classified into exactly ONE category per dimension. There are three scoring types:
- **Ordinal** — exact match = 1, adjacent category in the ordered list = 0.5, else 0. A catch-all such as "None of the above" scores 0 against any normal ideal.
- **Categorical** — fit is 1 if the joke's category equals the ideal, else 0.
- **Graded (intrinsic)** — Title Fit only; it is NOT compared to an ideal; the category itself maps directly to a score.

| # | Dimension | Type | Categories (in order) | Has ideal selector? | Classified by |
|---|-----------|------|-----------------------|---------------------|---------------|
| 1 | Length | Ordinal | Short, Medium, Long | Yes | **Code** (word count) |
| 2 | Topic | Categorical | (see 4.1 — provisional) | Yes | LLM |
| 3 | Humor Style | Categorical | Pun, Observational, Irony, Absurdity, Exaggeration, Self-deprecating, Anti-joke, Callback, None of the above | Yes | LLM |
| 4 | Complexity | Ordinal | Very simple, Simple, Moderate, Thoughtful, Expert | Yes | LLM |
| 5 | Edginess | Categorical | Clean, Slightly edgy, None of the above | Yes | LLM |
| 6 | Structure | Categorical | (see 4.1 — provisional, includes None of the above) | Yes | LLM |
| 7 | Wordplay | Ordinal | None, Light, Moderate, Heavy | Yes | LLM |
| 8 | Freshness | Ordinal | Timeless, Slightly current, Current, Very topical, Time-sensitive | Yes | LLM |
| 9 | Setup→Payoff | Ordinal | Immediate, Quick, Balanced, Long, Very long build | Yes | LLM |
| 10 | Clarity | Ordinal | Crystal clear, Mostly clear, Slightly ambiguous, Ambiguous, Reinterpretation | Yes | LLM |
| 11 | Energy | Ordinal | Deadpan, Low, Conversational, Animated, High-energy, None of the above | Yes | LLM |
| 12 | Title Fit | Graded | Perfect, Strong, Moderate, Weak, Mismatch | **No** (intrinsic) | LLM |

- **Title Fit has no ideal selector.** It measures how well the title matches the joke's actual theme, so it is scored on its own grade, not compared to a chosen ideal.
- **Length is classified by code**, not the LLM (see Section 5).

### 4.1 Provisional categories (Topic & Structure)
These two dimensions do not have finalized category lists yet — the sandbox exists partly to experiment with them. **Put all category lists in a single editable config file/constant** so the professor can tweak them between experiments. Suggested starting sets:

- **Topic (single word per category, no slashes):** `Work, Relationships, Family, Food, Technology, Animals, School, Money, Travel, Health, Sports, Politics, Everyday, Language, Other`
- **Structure (final list, given by professor):** `One-liner, Setup–punchline, Question–answer, Short story, Dialogue/conversation, List/build-up, None of the above`

> Treat these as adjustable. The app must not hardcode them in multiple places.

---

## 5. Length rule (code logic) — recommended

Classify Length by **word count** (simple, deterministic, language-agnostic enough for English jokes). Recommended thresholds (make them named constants so they're easy to tune):

```
words = number of whitespace-separated tokens in joke_text
Short   if words <= 15
Medium  if 16 <= words <= 40
Long    if words >= 41
```

- Rationale: a one-liner is typically ~15 words or fewer; a mid-length joke runs a couple of sentences; a "story" joke with a long build is 41+ words.
- Boundaries are non-overlapping: 15 -> Short, 40 -> Medium, 41 -> Long.
- Expose the two thresholds (`15`, `40`) as configurable constants.
- (Optional refinement to consider later: also factor in sentence count. Not required for v1.)

---

## 6. Scoring model (compute exactly like the main system)

### 6.1 Ordinal category order
Ordinal dimensions use their category order from Section 4 to decide adjacency. The order itself is the only thing that matters for scoring.

- **Length** (3): `Short, Medium, Long`
- **Wordplay** (4): `None, Light, Moderate, Heavy`
- **Complexity / Freshness / Setup→Payoff / Clarity / Energy** (5): categories in the order listed in Section 4

### 6.2 Per-dimension fit (`dim_fit`), range `[0,1]`
- **Ordinal:**
  - `1.0` if the joke's category exactly matches the ideal category
  - `0.5` if the joke's category is one step away from the ideal in the ordered category list
  - `0.0` otherwise
  - A catch-all such as `None of the above` is treated as binary: `1.0` only if the ideal is also the catch-all, otherwise `0.0`.
- **Categorical** (Topic, Humor Style, Edginess, Structure): `dim_fit = 1 if joke_category == ideal_category else 0`. A catch-all category follows the same rule.
- **Title Fit** (graded, intrinsic): `Perfect=1.0, Strong=0.75, Moderate=0.5, Weak=0.25, Mismatch=0`

### 6.3 Overall `true_fit`
```
true_fit = sum(dim_fit across all included dimensions)
```
- If all 12 dimensions are included, `true_fit` ranges `0–12`.
- **Title is required** (see Section 7 / 11), so Title Fit always contributes and the max is always `12`.
- Show `true_fit` both as a number and as a fraction of the max included (e.g. `9.66 / 12`).

> Note for context: in the main game, "Structure" is currently a placeholder and Title Fit is intrinsic. In THIS sandbox we include all dimensions so the professor can experiment with them. Keep an easy switch (a per-dimension `includeInTrueFit` flag in config) in case they want to exclude one.

---

## 7. App UX / screens

Single page, three areas (side-by-side on desktop, stacked on mobile):

### Panel A — Ideal Profile
- For each of the 11 dimensions with an ideal selector, a labeled dropdown (or segmented control) listing that dimension's categories.
- Sensible defaults pre-selected.
- Title Fit is shown here as informational text: "scored intrinsically, no ideal to set."

### Panel B — Test a Joke
- Textarea for `joke_text` (required).
- Text input for `title` (**required** — used to score Title Fit).
- "Classify" button — **disabled until both joke text and title are non-empty**; shows a loading state while the LLM call is in flight.

### Panel C — Results
- **Per-dimension table:** `Dimension | Classified category (LLM or code) | Ideal | dim_fit`, with a visual cue (e.g. color/bar) for how strong each `dim_fit` is.
  - Mark Length's row as "(code)" and Title Fit's row as "(intrinsic)".
- **Overall:** big `true_fit` number + `/ max` + a progress bar.
- **Raw JSON** panel: the exact JSON the model returned (collapsible/scrollable). The professor specifically wants to see this.

### Nice-to-have (recommended, low effort)
- **Recompute fit without re-calling the LLM when the ideal changes.** A joke's classification is intrinsic (independent of the ideal), so once you've classified a joke, changing an ideal dropdown should instantly recompute `dim_fit`/`true_fit` from the cached classification — no new API call. This is both cheaper and a nice demonstration of the model.

### States to handle
- Loading (LLM call in progress), error (LLM/network/parse failure with a readable message), empty (before first classify).

---

## 8. Backend

A minimal backend is required because the **OpenAI API key must stay server-side** (never expose it in the browser).

### 8.1 Endpoint
`POST /api/classify`
```jsonc
// request
{
  "joke_text": "I told my boss I needed a raise...",
  "title": "Corporate Comedy",           // required
  "ideal_profile": {                       // categories chosen by instructor
    "LENGTH": "Medium", "TOPIC": "Work", "HUMOR_STYLE": "Observational",
    "COMPLEXITY": "Moderate", "EDGINESS": "Clean", "STRUCTURE": "Setup–punchline",
    "WORDPLAY": "Light", "FRESHNESS": "Timeless", "SETUP_PAYOFF": "Balanced",
    "CLARITY": "Crystal clear", "ENERGY": "Conversational"
    // no TITLE_FIT — intrinsic
  }
}
```
```jsonc
// response
{
  "classification": {                      // the 11 LLM dims + code Length merged in
    "length": "Medium",                    // from code
    "topic": "Work",
    "humor_style": "Observational",
    "complexity": "Thoughtful",
    "edginess": "Clean",
    "structure": "Setup–punchline",
    "wordplay": "Moderate",
    "freshness": "Slightly current",
    "setup_payoff": "Balanced",
    "clarity": "Mostly clear",
    "energy": "Conversational",
    "title_fit": "Strong"
  },
  "dim_fits": {                            // per-dimension fit, 0..1
    "LENGTH": 1.0, "TOPIC": 1.0, "HUMOR_STYLE": 1.0, "COMPLEXITY": 0.75,
    "EDGINESS": 1.0, "STRUCTURE": 1.0, "WORDPLAY": 0.66, "FRESHNESS": 0.75,
    "SETUP_PAYOFF": 1.0, "CLARITY": 0.75, "ENERGY": 1.0, "TITLE_FIT": 0.75
  },
  "true_fit": 10.41,
  "max_fit": 12,
  "word_count": 24,
  "raw_llm_json": { /* exactly what the model returned, for display */ }
}
```
- The backend does the LLM call, computes Length + all fit math, and returns everything. (You may instead compute fit on the FE for the "recompute on ideal change" feature — if so, still return `classification` + `raw_llm_json`, and duplicate the small fit function on the client. Either is fine; keep one authoritative copy of the position/category constants.)

### 8.2 LLM integration
- **Model:** `gpt-4o-mini`, **temperature 0** (cheap, fast, deterministic).
- **Structured output:** use OpenAI structured outputs / JSON schema (or JSON mode) so the response is strictly parseable.
- **Classify only the 11 LLM dimensions** (exclude Length). Merge the code-derived Length into the final `classification` object afterward.
- Pass the **allowed categories** for each dimension in the prompt, and enforce them in the schema (use enums). The model must pick exactly one per dimension.

### 8.3 System prompt (starting point)
```
You are a precise joke classifier. You will be given a joke and its title
and a set of dimensions, each with an allowed list of categories. For EACH dimension,
choose EXACTLY ONE category from its allowed list that best describes the joke.
If none of the normal categories fit, choose "None of the above" for that dimension.
Do not invent categories. Respond ONLY with JSON matching the provided schema.

Judge "title_fit" as how well the title matches the joke's actual theme/subject.
```
Then provide the joke, the title, and the allowed categories per dimension.

### 8.4 JSON schema (11 LLM dims, single joke)
Each property is an enum constrained to that dimension's category list, e.g.:
```jsonc
{
  "type": "object",
  "properties": {
    "topic":        { "type": "string", "enum": ["Work","Relationships","Family","Food","Technology","Animals","School","Money","Travel","Health","Sports","Politics","Everyday","Language","Other"] },
    "humor_style":  { "type": "string", "enum": ["Pun","Observational","Irony","Absurdity","Exaggeration","Self-deprecating","Anti-joke","Callback","None of the above"] },
    "complexity":   { "type": "string", "enum": ["Very simple","Simple","Moderate","Thoughtful","Expert"] },
    "edginess":     { "type": "string", "enum": ["Clean","Slightly edgy","None of the above"] },
    "structure":    { "type": "string", "enum": ["One-liner","Setup–punchline","Question–answer","Short story","Dialogue/conversation","List/build-up","None of the above"] },
    "wordplay":     { "type": "string", "enum": ["None","Light","Moderate","Heavy"] },
    "freshness":    { "type": "string", "enum": ["Timeless","Slightly current","Current","Very topical","Time-sensitive"] },
    "setup_payoff": { "type": "string", "enum": ["Immediate","Quick","Balanced","Long","Very long build"] },
    "clarity":      { "type": "string", "enum": ["Crystal clear","Mostly clear","Slightly ambiguous","Ambiguous","Reinterpretation"] },
    "energy":       { "type": "string", "enum": ["Deadpan","Low","Conversational","Animated","High-energy","None of the above"] },
    "title_fit":    { "type": "string", "enum": ["Perfect","Strong","Moderate","Weak","Mismatch"] }
  },
  "required": ["topic","humor_style","complexity","edginess","structure","wordplay","freshness","setup_payoff","clarity","energy","title_fit"],
  "additionalProperties": false
}
```

---

## 9. Tech stack recommendation

Recommended (simplest single app, keeps the API key server-side):
- **Next.js (React + a single API route)** — one repo, one `npm run dev`, the `/api/classify` route holds the OpenAI call. Use the official `openai` npm package.

Acceptable alternatives:
- **Vite + React** front + a tiny **FastAPI (Python)** or **Express (Node)** backend exposing `/api/classify`.

Whatever the choice: React for the UI, a minimal server for the LLM call, no database.

---

## 10. Config / environment
- `OPENAI_API_KEY` — required, server-side only (e.g. `.env.local`, never shipped to the browser).
- `OPENAI_MODEL` — default `gpt-4o-mini`.
- A single `dimensions.ts`/`dimensions.py` config module holding: category lists (including provisional Topic/Structure), ordinal position maps, Title Fit grade map, Length thresholds, and per-dimension `type` + `includeInTrueFit` flags. This is the single source of truth.

---

## 11. Validation & edge cases
- **Empty joke_text** → block submit with a message.
- **Empty title** → block submit with a message (title is required, same as joke text). The Classify button stays disabled until both fields are filled.
- **LLM returns a category not in the allowed list** → validate server-side against the config; if invalid, retry once, then surface a clear error. (Structured outputs with enums should prevent this, but validate anyway.)
- **JSON parse / network / rate-limit errors** → return a readable error to the FE; FE shows it without crashing.
- **Determinism note:** temperature 0 makes results largely stable but not guaranteed identical every time; that's expected.

---

## 12. Worked example (for your sanity check)

Ideal: `Length=Medium, Topic=Work, Humor Style=Observational, Complexity=Moderate, Edginess=Clean, Structure=Setup–punchline, Wordplay=Light, Freshness=Timeless, Setup→Payoff=Balanced, Clarity=Crystal clear, Energy=Conversational`.

Joke classified as: `Length=Medium(exact), Topic=Work(match), Humor Style=Observational(match), Complexity=Thoughtful(adjacent), Edginess=Clean(match), Structure=Setup–punchline(match), Wordplay=Moderate(adjacent), Freshness=Slightly current(adjacent), Setup→Payoff=Balanced(exact), Clarity=Mostly clear(adjacent), Energy=Conversational(exact), Title Fit=Strong`.

dim_fits: `1.0, 1.0, 1.0, 0.5, 1.0, 1.0, 0.5, 0.5, 1.0, 0.5, 1.0, 0.75`
`true_fit = 9.75 / 12`.

---

## 13. Acceptance criteria
- [ ] Instructor can set an ideal category for all 11 selectable dimensions; Title Fit shown as intrinsic.
- [ ] Submitting a joke calls the LLM (server-side) and returns a valid, schema-conformant classification.
- [ ] Length is computed by code (word count) and shown as such.
- [ ] The UI shows, per dimension: the classified category, the ideal, and `dim_fit`.
- [ ] The UI shows overall `true_fit` (with max) and the raw model JSON.
- [ ] Submit is blocked (button disabled) unless BOTH joke text and title are provided; Title Fit always contributes to the total.
- [ ] Category lists / thresholds / positions live in one editable config module.
- [ ] (Nice-to-have) Changing an ideal after classifying recomputes fit without a new LLM call.
