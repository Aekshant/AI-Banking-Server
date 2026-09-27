CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE bank_customers (
    customer_id VARCHAR(64) PRIMARY KEY,
    full_name VARCHAR(128) NOT NULL,
    pan_number VARCHAR(10) UNIQUE NOT NULL,
    aadhaar_number VARCHAR(12) UNIQUE NOT NULL,
    account_balance NUMERIC(15, 2) NOT NULL DEFAULT 0,
    credit_score INT NOT NULL,
    is_2fa_verified BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE transactions (
    transaction_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key VARCHAR(128) UNIQUE NOT NULL,
    from_account VARCHAR(64) NOT NULL,
    to_account VARCHAR(64) NOT NULL,
    amount NUMERIC(15, 2) NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),

    CONSTRAINT fk_from_account
        FOREIGN KEY (from_account)
        REFERENCES bank_customers(customer_id),

    CONSTRAINT fk_to_account
        FOREIGN KEY (to_account)
        REFERENCES bank_customers(customer_id)
);
