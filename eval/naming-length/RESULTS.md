# Naming-section length A/B — results

## Setup

- 5 length variants of the inline `## Naming` section in `SKILL.md`.
- All variants point to the same lazy-loaded `naming.md` (681 words: replacement protocol, heuristics table, 5 bad→good pairs, uniform-tree example).
- 5 fixtures × 1 run × 5 variants = 25 subagent invocations, each with clean context.
- Each subagent received the variant inline text + access to `naming.md` + fixture, and emitted a structured PASS/FLAG verdict per `s.Test(...)` literal.
- Ground truth: 20 verdicts total (13 FLAG, 7 PASS) per fixture set, in `fixtures/expectations.json`.

## Variant sizes

| Variant | Words | Description |
|---|---|---|
| v1 | 206 | Full Variant A naming section (rule + 4 forbidden patterns with regex hints + worked examples). |
| v2 | 129 | Trimmed: rule + bulleted forbidden patterns with 1 example each. |
| v3 | 49 | Compressed: rule + parenthesized forbidden examples. |
| v4 | 31 | Two-sentence: "names are business actions/outcomes... not function names, HTTP codes, assertion verbs, data-shape words". |
| v5 | 16 | One-liner: "`s.Test(...)` names are business actions, never function names, status codes, or assertions." |

All variants end with a pointer to `naming.md`.

## Scores

| Variant | Correct verdicts | FP on controls | FN on FLAGs |
|---|---|---|---|
| v1 | 20/20 | 0 | 0 |
| v2 | 20/20 | 0 | 0 |
| v3 | 20/20 | 0 | 0 |
| v4 | 20/20 | 0 | 0 |
| v5 | 20/20 | 0 | 0 |

Replacement quality (subjective, eyeballed across 65 FLAG blocks): consistently business-level across all variants — `"updating a quote writes an audit record"`, `"rejects duplicate provider name"`, `"borrower has no past loans"`, `"admin creates a new provider"`. No variant produced word-shuffle replacements.

## Interpretation — caveat

The eval **does not discriminate**. Even the 16-word v5 matched the 206-word v1 perfectly. Two non-exclusive explanations:

1. **Lazy-loaded `naming.md` is doing the work.** Every variant points at it, and a one-shot eval with no context cost incentivises the subagent to load the reference unconditionally. So the eval measured "any pointer + full naming.md", not the inline section's own carrying capacity.
2. **The task is too easy.** The 5 fixtures use textbook violations (literal `CreateQuote`, literal `returns 409`, literal `err is not nil`, literal `with empty`). A one-line cue plus the user's prior knowledge of naming conventions is enough.

The eval cannot tell which dominates without a follow-up run that suppresses the lazy load.

## What would actually discriminate

Re-run with **no access to `naming.md`** — only the inline variant text. Then v5 ("never function names, status codes, or assertions") is forced to carry the whole load. Predictions if that eval runs:

- v1/v2 still ≈ 20/20 (self-contained).
- v3 likely 18–20 (compressed but mentions all 4 patterns).
- v4 likely 16–19 (drops the protocol — replacement quality may degrade).
- v5 likely 14–18 (no structure-leak language; "with empty" / "with nil filter" / "map with 0 entries" may slip through as PASS).

## Recommendation

**Ship a v3-class section** in `SKILL.md` (~50 words inline) + the full `naming.md` lazy-loaded. Rationale:

- v3 names all 4 forbidden patterns with one literal example each — enough to trigger the lazy load on a match.
- v1's worked examples duplicate `naming.md`. Net cost to every samurai-skill activation.
- v4/v5 read as catchy one-liners but drop the structure-phrasing cue (the hardest category — "empty / nil / with N" doesn't pattern-match as obviously as `CreateQuote` or `409`).

Concrete v3 inline:

> ## Naming
>
> Every `s.Test("name", ...)` — at every tree level — names a business action or outcome. Forbidden: identifier leak (`UpdateQuote calls...`), HTTP codes (`returns 409`), assertion phrasing (`err is not nil`), structure phrasing (`with empty slice`). Flag matches; read the `Test()` body; propose 1–2 domain replacements. See [naming.md](naming.md).

## Follow-up

Before shipping, run the suppressed-load eval to confirm v3 holds without `naming.md`. If v3 degrades, fall back to v2. If even v5 holds (unlikely), the inline section can be reduced further.
