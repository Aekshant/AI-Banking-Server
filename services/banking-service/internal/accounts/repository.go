package accounts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("account not found")

// Customer is a bank_customers row with unmasked PII. It must never be
// returned to clients directly; use ToProfile instead.
type Customer struct {
	CustomerID    string
	FullName      string
	PAN           string
	Aadhaar       string
	Balance       string // NUMERIC as text, to avoid float rounding
	CreditScore   int
	Is2FAVerified bool
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(ctx context.Context, id string) (*Customer, error) {
	var c Customer
	err := r.db.QueryRow(ctx, `
		SELECT customer_id, full_name, pan_number, aadhaar_number,
		       account_balance::text, credit_score, COALESCE(is_2fa_verified, FALSE)
		FROM bank_customers
		WHERE customer_id = $1`, id,
	).Scan(&c.CustomerID, &c.FullName, &c.PAN, &c.Aadhaar,
		&c.Balance, &c.CreditScore, &c.Is2FAVerified)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}
