"""Seed bank_customers with 50 synthetic Indian customers.

All values are fake and generated with Faker. Re-running is safe: rows are
upserted by customer_id, and a fixed random seed keeps the data stable.

Database settings come from the environment or the repo's .env file:
DATABASE_URL, or POSTGRES_USER / POSTGRES_PASSWORD / POSTGRES_DB (+ optional
POSTGRES_HOST / POSTGRES_PORT / POSTGRES_SSLMODE).

Usage:
    pip install -r requirements.txt
    python seed.py
"""

import os
import random
from decimal import Decimal

import psycopg
from dotenv import find_dotenv, load_dotenv
from faker import Faker

NUM_CUSTOMERS = 50
SEED = 42


LETTERS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
DIGITS = "0123456789"

fake = Faker("en_IN")


def generate_pan(surname):
    # Format: AAAPS9999A. The 4th char is the holder type ("P" = individual)
    # and the 5th is the first letter of the surname.
    prefix = "".join(random.choices(LETTERS, k=3))
    numbers = "".join(random.choices(DIGITS, k=4))
    last = random.choice(LETTERS)
    return f"{prefix}P{surname[0].upper()}{numbers}{last}"


def generate_aadhaar():
    # Real Aadhaar numbers never start with 0 or 1.
    return random.choice("23456789") + "".join(random.choices(DIGITS, k=11))


def generate_customers():
    pans, aadhaars = set(), set()
    customers = []

    for i in range(1, NUM_CUSTOMERS + 1):
        first, last = fake.first_name(), fake.last_name()

        pan = generate_pan(last)
        while pan in pans:
            pan = generate_pan(last)
        pans.add(pan)

        aadhaar = generate_aadhaar()
        while aadhaar in aadhaars:
            aadhaar = generate_aadhaar()
        aadhaars.add(aadhaar)

        customers.append(
            (
                f"customer-{i:03d}",
                f"{first} {last}",
                pan,
                aadhaar,
                Decimal(random.randint(1_000_00, 5_00_000_00)) / 100,
                random.randint(300, 900),
                random.random() < 0.7,
            )
        )

    return customers


def connection_params():
    """Returns kwargs for psycopg.connect, read from the environment."""
    load_dotenv(find_dotenv(usecwd=True))  # never overrides existing variables

    if os.getenv("DATABASE_URL"):
        return {"conninfo": os.environ["DATABASE_URL"]}

    required = ["POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"]
    missing = [k for k in required if not os.getenv(k)]
    if missing:
        raise SystemExit(
            f"Missing environment variables: {', '.join(missing)}. "
            "Copy .env.example to .env at the repo root, or set DATABASE_URL."
        )

    return {
        "user": os.environ["POSTGRES_USER"],
        "password": os.environ["POSTGRES_PASSWORD"],
        "dbname": os.environ["POSTGRES_DB"],
        "host": os.getenv("POSTGRES_HOST", "localhost"),
        "port": os.getenv("POSTGRES_PORT", "5432"),
        "sslmode": os.getenv("POSTGRES_SSLMODE", "disable"),
    }


def main():
    random.seed(SEED)
    Faker.seed(SEED)

    customers = generate_customers()

    with psycopg.connect(**connection_params()) as conn:
        conn.cursor().executemany(
            """
            INSERT INTO bank_customers (
                customer_id, full_name, pan_number, aadhaar_number,
                account_balance, credit_score, is_2fa_verified
            )
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            ON CONFLICT (customer_id) DO UPDATE SET
                full_name = EXCLUDED.full_name,
                pan_number = EXCLUDED.pan_number,
                aadhaar_number = EXCLUDED.aadhaar_number,
                account_balance = EXCLUDED.account_balance,
                credit_score = EXCLUDED.credit_score,
                is_2fa_verified = EXCLUDED.is_2fa_verified
            """,
            customers,
        )

    print(f"Seeded {len(customers)} customers.")


if __name__ == "__main__":
    main()
