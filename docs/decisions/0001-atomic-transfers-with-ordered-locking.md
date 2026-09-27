# 0001. Atomic transfers with ordered locking

- **Status:** Accepted
- **Date:** 2026-09-27

## Context

A transfer changes two balances and records a transaction. If these happen as independent statements, a crash or error part-way through leaves money debited but never credited. Concurrent transfers add two more risks:

- **Double spend:** two transfers both read a balance of ₹10,000 and both debit ₹8,000.
- **Deadlock:** a transfer A→B locks A then waits for B, while a transfer B→A locks B then waits for A.

An AI agent makes concurrency more likely, not less: it may issue tool calls in parallel.

## Decision

Every transfer runs in a single database transaction:

```
BEGIN
  1. lock both accounts, ordered by customer_id   (SELECT ... FOR NO KEY UPDATE)
  2. idempotency check                             (see ADR 0002)
  3. check balance
  4. debit source
  5. credit destination
  6. insert transaction
COMMIT        -- any error before this point -> ROLLBACK
```

- **Locks are taken in `customer_id` order**, not source-then-destination. With a global order, two transfers over the same pair of accounts always request locks in the same sequence, so neither can hold one lock while waiting for the other.
- **`FOR NO KEY UPDATE`** rather than `FOR UPDATE`: we change balances, not keys. `FOR UPDATE` would conflict with the `FOR KEY SHARE` locks that the foreign-key checks in step 6 take, and could deadlock.
- **The balance is checked while the row is locked**, so it cannot change between the check and the debit.

## Consequences

- A transfer is applied completely or not at all.
- Transfers touching the same account are serialised. Throughput on a single hot account is limited, which is acceptable for per-customer accounts and would need revisiting for, say, a merchant account receiving thousands of payments per second.
- Verified locally: 100 concurrent transfers in both directions between two accounts completed with no deadlocks, and the total money in the system was unchanged.
- Failed transfers (insufficient funds, unknown account) are rolled back and leave no record. An audit trail of failures is on the roadmap.
