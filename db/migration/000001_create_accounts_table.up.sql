CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE "account_type" AS ENUM (
    'Asset',
    'Liability',
    'Equity',
    'Revenue',
    'Expense'
);

CREATE TYPE "normal_balance" AS ENUM (
    'Debit',
    'Credit'
);

CREATE TABLE accounts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(100) UNIQUE NOT NULL DEFAULT substring(gen_random_uuid()::text FROM 1 FOR 10),
    name        VARCHAR(100) NOT NULL,
    type        "account_type" NOT NULL,
    category    VARCHAR(100) NOT NULL,
    normal_balance  "normal_balance" NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);