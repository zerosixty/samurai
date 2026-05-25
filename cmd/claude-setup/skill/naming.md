# Samurai Test Naming

The "Naming" section in SKILL.md states the bar and the four forbidden patterns. This file is the replacement protocol and worked examples — load it when you have a bad name in hand and need to propose a good one.

## Replacement protocol

Do not look at the diff hunk header, the parent test function name, or the function under test. **Open the `Test()` callback body.** Identify three things from the code in that body:

1. **Actor** — who acts. Usually a domain noun: `borrower`, `admin`, `provider`, `API caller`, `counterparty`. If there is no clear actor, the system itself (`the service`, `the API`) is acceptable but rarely the best choice.
2. **Verb** — the action call that mutates state or makes a request: `db.CreateUser`, `client.UpdateQuote`, `svc.Settle`. Express it as a domain verb (`creates`, `updates`, `settles`), not the Go method name.
3. **Outcome** — what the assertions in the same callback verify: a state change, a rejection, a returned value's domain meaning.

Write a 3–7 word, lowercase, present-tense phrase combining (1)+(2)+(3) — or (1)+(2) when the outcome is implied by success. Propose 1–2 alternatives so the author can pick.

If you cannot identify the actor or the outcome from the body, the `Test()` is doing too much or too little — flag this before renaming. Either the action is missing (a setup-only test) or many actions are bundled (split it).

## Detection heuristics

| Pattern | Why it's bad | What to look at in the body instead |
|---|---|---|
| CamelCase token matching a Go identifier in the Test body | Couples the test name to the SUT symbol; rename of the function breaks the name | The effect of that call in domain terms |
| Bare HTTP status code (`200 → accepts`, `404 → reports missing`, `409 → rejects duplicate id`) | Leaks the HTTP layer; the same behaviour at gRPC would be unnamable | The outcome verb the code stands for, not the code itself |
| `is / equals / returns / not nil / has length / contains` describing a value | Describes the assertion, not the step | The action that produced the value; the assertion is implementation |
| `empty / nil / with N keys / map / slice / struct` | Leaks the input data structure | The domain meaning of the input: `no past loans`, `missing preferences`, `unset filter` |

## Bad → good (grounded in real samurai trees)

- `"UpdateQuote calls produce audit records with matching client quote ids"` — identifier leak (`UpdateQuote`)
  → `"updating a quote writes an audit record"`
  → `"the audit record links to the originating client quote"`

- `"err is not nil"` — assertion phrasing
  → `"rejects request with invalid signature"`
  → `"refuses unauthenticated callers"`

- `"returns 409"` — HTTP leak
  → `"rejects duplicate id"`
  → `"refuses to create a provider that already exists"`

- `"with empty slice"` — structure leak
  → `"returns no matches when the filter is unset"`
  → `"borrower has no past loans"`

- `"CreateProvider returns nil"` — identifier leak + assertion phrasing
  → `"creates a new provider successfully"`
  → `"a fresh provider id is issued"`

## Uniformly across tree levels

The bar is the same at top, intermediate, and leaf. Example:

```
"borrower opens a credit line"           // top — business setup, not "CreateCreditLine returns ok"
  ├─ "borrower draws on the credit line" // intermediate — action, not "Draw=true"
  │    ├─ "balance reflects the drawdown"  // leaf — outcome, not "balance equals 100"
  │    └─ "draw is rejected when limit is exceeded"
  └─ "counterparty accepts KYB access"
```

No level mentions the Go type (`CreditLine`), the method (`Draw`), the HTTP code, the assertion verb (`equals`), or the data shape (`with balance > 0`).

## During review

When reviewing a samurai test, scan every `s.Test(...)` string literal against the heuristics above. For each hit:

1. Quote the offending name and identify which forbidden pattern it matches.
2. Read the `Test()` body and derive actor + verb + outcome.
3. Propose 1–2 concrete replacements.

Do not silently rewrite — surface the issue and the proposal so the author confirms domain accuracy.
