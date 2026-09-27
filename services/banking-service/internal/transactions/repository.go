package transactions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const StatusSettled = "SETTLED"

var (
	ErrAccountNotFound     = errors.New("account not found")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different parameters")
)

type Transaction struct {
	TransactionID string      `json:"transaction_id" example:"3f1c2a9e-8b7d-4c6e-9f10-2a3b4c5d6e7f"`
	Status        string      `json:"status" example:"SETTLED"`
	Amount        json.Number `json:"amount" swaggertype:"number" example:"5000"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Transfer moves amount (a validated decimal string) between two accounts as
// a single database transaction:
//
//	BEGIN
//	  1. lock source and destination accounts
//	  2. idempotency check: key already used -> return the original transaction
//	  3. check balance
//	  4. debit source
//	  5. credit destination
//	  6. insert transaction
//	COMMIT
//
// Any error before COMMIT rolls everything back, so a transfer is applied
// completely or not at all. replayed is true when the original transaction
// for idempotencyKey is returned instead of moving money again.
func (r *Repository) Transfer(ctx context.Context, from, to, amount, idempotencyKey string) (t *Transaction, replayed bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	// ROLLBACK on every early return; a no-op once COMMIT has succeeded.
	defer tx.Rollback(ctx)

	// 1. Lock both accounts. They are locked in customer_id order, not
	// source-then-destination: otherwise A->B and B->A running together would
	// each hold one lock and wait forever for the other (deadlock).
	// NO KEY UPDATE still lets step 6's foreign-key checks through.
	rows, err := tx.Query(ctx, `
		SELECT customer_id, account_balance >= $2::numeric
		FROM bank_customers
		WHERE customer_id = ANY($1)
		ORDER BY customer_id
		FOR NO KEY UPDATE`, []string{from, to}, amount)
	if err != nil {
		return nil, false, err
	}
	hasFunds := map[string]bool{}
	var id string
	var ok bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &ok}, func() error {
		hasFunds[id] = ok
		return nil
	}); err != nil {
		return nil, false, err
	}
	if len(hasFunds) != 2 {
		return nil, false, ErrAccountNotFound
	}

	// 2. Idempotency check, done after locking: a retry racing the original
	// request waits in step 1 until the original commits, then finds it here.
	existing, err := findByKey(ctx, tx, from, to, amount, idempotencyKey)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	if existing != nil {
		return existing, true, nil
	}

	// 3. Check balance. The row is locked, so it cannot change before step 4.
	if !hasFunds[from] {
		return nil, false, ErrInsufficientFunds
	}

	// 4. Debit source.
	if _, err := tx.Exec(ctx, `
		UPDATE bank_customers
		SET account_balance = account_balance - $1::numeric
		WHERE customer_id = $2`, amount, from); err != nil {
		return nil, false, err
	}

	// 5. Credit destination.
	if _, err := tx.Exec(ctx, `
		UPDATE bank_customers
		SET account_balance = account_balance + $1::numeric
		WHERE customer_id = $2`, amount, to); err != nil {
		return nil, false, err
	}

	// 6. Insert transaction. The UNIQUE idempotency_key is the final guard: if
	// a request with the same key but different accounts committed since
	// step 2, nothing is inserted, and we roll back steps 4-5.
	var txID, status, amt string
	err = tx.QueryRow(ctx, `
		INSERT INTO transactions (idempotency_key, from_account, to_account, amount, status)
		VALUES ($1, $2, $3, $4::numeric, $5)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING transaction_id::text, status, amount::text`,
		idempotencyKey, from, to, amount, StatusSettled,
	).Scan(&txID, &status, &amt)
	if errors.Is(err, pgx.ErrNoRows) {
		tx.Rollback(ctx)
		existing, err := findByKey(ctx, r.db, from, to, amount, idempotencyKey)
		if err != nil {
			return nil, false, err
		}
		return existing, true, nil
	}
	if err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return &Transaction{TransactionID: txID, Status: status, Amount: json.Number(amt)}, false, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// findByKey returns the transaction stored under idempotencyKey, or
// ErrIdempotencyConflict if it was created for a different transfer.
func findByKey(ctx context.Context, q querier, from, to, amount, idempotencyKey string) (*Transaction, error) {
	var id, status, amt string
	var matches bool
	err := q.QueryRow(ctx, `
		SELECT transaction_id::text, status, amount::text,
		       from_account = $2 AND to_account = $3 AND amount = $4::numeric
		FROM transactions
		WHERE idempotency_key = $1`,
		idempotencyKey, from, to, amount,
	).Scan(&id, &status, &amt, &matches)
	if err != nil {
		return nil, err
	}
	if !matches {
		return nil, ErrIdempotencyConflict
	}
	return &Transaction{TransactionID: id, Status: status, Amount: json.Number(amt)}, nil
}
