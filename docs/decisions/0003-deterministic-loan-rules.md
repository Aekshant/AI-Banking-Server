# 0003. Deterministic loan rules, no LLM

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

The platform will have an AI agent, and it is tempting to let the model decide loan eligibility. But credit decisions must be:

- **Reproducible:** the same inputs always produce the same answer.
- **Auditable:** we can show exactly why a customer was rejected.
- **Testable:** boundaries are checked by unit tests, not by prompting.

An LLM satisfies none of these reliably.

## Decision

`POST /api/v1/loans/evaluate` uses plain rules in `internal/loans/rules.go`, evaluated in order, where the first failure decides:

| # | Rule | Reason |
|---|---|---|
| 1 | Credit score ≥ 650 | `LOW_CREDIT_SCORE` |
| 2 | 2FA verified | `IDENTITY_NOT_VERIFIED` |
| 3 | Amount ≤ min(balance × multiplier, ₹1 crore) | `AMOUNT_EXCEEDS_LIMIT` |

The multiplier is 10× for a score of 750+, 5× for 700–749 and 2× for 650–699. The response includes `max_eligible_amount` so the agent can explain a rejection without guessing.

The rules are a pure function with no database or HTTP access.

## Consequences

- The agent *explains* decisions; code *makes* them.
- The thresholds are placeholders, not a real credit policy, and are expected to change.
- Table-driven tests cover every boundary, including one paisa over the limit.
