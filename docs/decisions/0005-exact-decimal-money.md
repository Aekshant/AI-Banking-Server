# 0005. Exact decimal money, no floats

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

Floating-point numbers cannot represent most decimal amounts exactly: `0.1 + 0.2 != 0.3`. In a ledger, these errors accumulate and balances drift.

## Decision

Money is never a `float64` anywhere in the request path:

- **Storage:** `NUMERIC(15, 2)` in Postgres.
- **Reads:** balances are selected as text (`account_balance::text`) and returned as `json.Number`, so `125000.50` is sent exactly as stored.
- **Requests:** amounts are decoded as `json.Number` and validated as text: positive, at most 2 decimals, at most 13 integer digits (`internal/money`).
- **Arithmetic:** transfers do the maths in SQL (`account_balance - $1::numeric`). Loan limits use `math/big.Rat`.

## Consequences

- Balances are exact to the paisa.
- Amounts with 3 or more decimals, or in scientific notation (`1e3`), are rejected with `400` rather than rounded.
- JSON responses show 2 decimals (`5000.00`) because that is what Postgres stores.
