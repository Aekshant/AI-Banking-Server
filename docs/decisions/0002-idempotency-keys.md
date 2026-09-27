# 0002. Idempotency keys for transfers

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

Clients retry. A request can succeed on the server while the response is lost to a timeout, and the client cannot tell the difference between "failed" and "succeeded but I never heard back". An LLM agent retries even more readily. Without protection, a retried `transfer ₹5,000` sends another ₹5,000.

## Decision

Every transfer request carries a client-generated `idempotency_key`, stored in `transactions.idempotency_key VARCHAR(128) UNIQUE`.

- If the key has been used **for the same transfer** (same accounts and amount), return the original transaction with `200` and move no money.
- If the key has been used **for a different transfer**, reject it with `409 Conflict`. Silently returning an unrelated transaction would hide a client bug.
- A new transfer returns `201`.

The check runs **after** the account locks are taken (ADR 0001). A retry that races the original waits on the lock, then sees the committed transaction. Checking before locking would let both requests through.

The `UNIQUE` constraint is the final guard. In the one race the check cannot see (same key, different accounts, so different locks), the insert in step 6 finds the conflict, and the debit and credit are rolled back.

## Consequences

- Retries are safe, which is essential once an agent calls this API as a tool.
- Verified locally: 20 concurrent identical requests produced one `201`, nineteen `200`s and exactly one transaction row.
- Keys are stored forever. A retention policy (for example, keys expire after 24 hours) may be needed at scale.
- Only successful transfers claim a key. A failed transfer can be retried with the same key once the cause (such as insufficient funds) is fixed.
