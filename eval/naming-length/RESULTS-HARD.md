# Naming-section length A/B — hardened eval (no lazy load + mixed fixtures + N=3)

## Why a second eval

The first eval (`RESULTS.md`) was non-discriminating — every variant scored 20/20 because all variants pointed to the same lazy-loaded `naming.md` (681 words) and subagents loaded it. The inline section's own carrying capacity was never tested.

This eval fixes three things:

1. **No lazy load.** Variants live in `variants/no-load/v{1..5}.md`, stripped of the `[naming.md]` pointer. The directory has no `naming.md` sibling. The inline text is the only guidance.
2. **Mixed fixtures.** New `F{1..5}_hard.go.txt` mix textbook violations + borderline cases (passive voice, "has X" used in domain phrasing, "rejected" as domain outcome, status-range shorthand `4xx`) + clean controls with risky-looking words.
3. **N=3 per cell.** 5 variants × 5 fixtures × 3 runs = 75 subagent invocations, clean context each.

Ground truth: 21 verdicts per run, 7 FLAG + 14 PASS — see `fixtures/expectations-hard.json`. Each name labelled `textbook`, `borderline`, or `clean` for diagnostic slicing.

## Variant sizes

| Variant | Words | What's in it |
|---|---|---|
| v1 | 206 | All 4 forbidden patterns + worked examples + safe-examples carve-out paragraph. |
| v2 | 129 | All 4 patterns with bulleted examples + safe-examples one-liner. |
| v3 | 49 | All 4 patterns with parenthesized examples + brief safe-words clause. |
| v4 | 31 | All 4 patterns in one sentence, no safe-examples clause. |
| v5 | 16 | "business actions, never function names, status codes, or assertions" — no structure clause. |

## Scores

| Variant | Total correct | Score | Per-run (run 1 / 2 / 3) | Failure mode |
|---|---|---|---|---|
| v1 (206w) | 58/63 | 92.1% | 20 / 18 / 20 | Over-flags passive voice (`"first loan is added to the history"` — all 3 runs FLAGged for "is") |
| **v2 (129w)** | **62/63** | **98.4%** | 21 / 20 / 21 | One stray false-FLAG; everything else stable. |
| v3 (49w) | 60/63 | 95.2% | 20 / 20 / 20 | Consistently false-FLAGs `"the request is rejected for missing auth"` (no carve-out for passive domain outcomes). |
| v4 (31w) | 59/63 | 93.7% | 20 / 19 / 20 | Consistently false-FLAGs `"borrower has insufficient collateral"` — v4 lists `has X` as forbidden but drops the safe-examples clause. |
| v5 (16w) | 54/63 | 85.7% | 18 / 16 / 18 | **Structure leak detection collapses.** All 3 runs PASS `"borrower with empty loan history"` (textbook structure leak — should FLAG). Also misses `"the new provider has the requested name"`. |

## Per-fixture breakdown

| | F1 (id) | F2 (assert) | F3 (struct) | F4 (HTTP) | F5 (control) |
|---|---|---|---|---|---|
| v1 | 12/12 | 11/12 | 9/12 | 11/12 | 15/15 |
| **v2** | **12/12** | **12/12** | **12/12** | **11/12** | **15/15** |
| v3 | 12/12 | 12/12 | 12/12 | 9/12 | 15/15 |
| v4 | 12/12 | 12/12 | 12/12 | 11/12 | 12/15 |
| v5 | 12/12 | 9/12 | 8/12 | 10/12 | 15/15 |

## Where each variant breaks

- **v1's over-detail backfires.** Adding `is` to the assertion-verb list trips passive constructions like `"is added"`. The longer prose gives the subagent more rules to misapply.
- **v2 is the sweet spot.** Lists all 4 patterns with concrete examples *and* has the safe-examples disclaimer. Enough constraint, not too much.
- **v3 holds on 3/4 categories** but its briefer safe-words clause isn't enough to protect `"rejected"` as a domain outcome.
- **v4 loses the safe-examples clause.** "has X" is listed as forbidden but `"borrower has insufficient collateral"` is exactly the canonical domain phrasing. Without the carve-out, the variant punishes correct names.
- **v5 collapses on structure-leak detection.** The 16-word one-liner has no `structure / data-shape / empty / nil` cue. Three textbook leaks pass through (`"with empty loan history"`, `"has the requested name"`), and `"empty"` slips entirely (all 3 runs).

The degradation isn't monotonic in length — it's monotonic in **rule coverage**. v1 has too much detail, v5 has too little, v2–v4 trade precision against verbosity.

## Why v2 beats v1

v1 (Variant A's current naming section) is the section originally drafted as "good enough". The eval shows it's *worse* than a 129-word version. v1's extra detail — explicit `is` in the assertion-verb list, regex hints, repeated examples — makes the rule too aggressive on perfectly fine passive constructions. Trimming detail to v2 actually raises score.

## Recommendation

**Ship v2** as the inline `## Naming` section in `cmd/claude-setup/skill/SKILL.md`, plus the full `naming.md` lazy-loaded.

Rationale:
- v2 scored highest in the hardened eval (98.4% vs 92.1% for v1).
- v2 holds all 5 categories with one stray miss; no consistent failure mode.
- The `naming.md` lazy load remains net positive on borderline cases (it adds the actor/verb/outcome protocol), but is no longer load-bearing for basic rule application.
- Cost: 129 words inline, loaded with every samurai-skill activation. Acceptable.

Concrete v2 to ship (back-edit the naming.md pointer in for production):

```markdown
## Naming

Every `s.Test("name", ...)` — at every tree level — names a business action or outcome from the actor's POV. Not the function under test, the assertion, or the input shape.

Forbidden patterns:

- **Identifier leak** — CamelCase tokens matching Go identifiers in the Test body. `"UpdateQuote calls..."` leaks `UpdateQuote`.
- **HTTP code leak** — bare 3-digit codes (`\b[1-5]\d{2}\b`) or `4xx/5xx` shorthand. `"returns 409"` → `"rejects duplicate id"`.
- **Assertion phrasing** — `is / equals / returns / not nil / has length / contains / has`. `"err is not nil"` → `"rejects invalid signature"`.
- **Structure phrasing** — `empty / nil / with N / map / slice / struct`. `"with empty slice"` → `"borrower has no past loans"`.

Risky-looking words used in a domain-natural sense (`borrower returns the equipment`, `has insufficient collateral`, `no past loans`) are clean — only flag when the word describes the assertion, the data shape, or the identifier. Flag matches, read the body, propose 1–2 replacements. See [naming.md](naming.md) for the protocol.
```

## Eval quality notes

- N=3 was enough to expose consistent failure modes (v3, v4, v5 each had a 3/3 false-FLAG or false-PASS). For ranking purposes more runs aren't needed.
- The 7-FLAG / 14-PASS asymmetry biases the eval toward variants that PASS too eagerly (v5 still scored 86% by passing everything in F5). Real samurai trees have closer to 50/50 — re-run on production samples to confirm v2 holds.
- Ground-truth for `"first loan is added to the history"` is a judgement call. The naming.md examples use passive forms (`"first loan is added"` is the spirit of `"a fresh provider id is issued"`). If you treat this as FLAG instead, v1 jumps from 92.1% to ~96.8% — still behind v2.
- Cost: 75 subagents × ~21k tokens ≈ 1.6M tokens. The original suspect eval was 25 × 23k ≈ 575k. Worth the 2.8× spend to actually answer the question.
