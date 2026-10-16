ALTER TYPE entry_kind ADD VALUE IF NOT EXISTS 'borrowing';   -- loan money received (or a purchase on credit)
ALTER TYPE entry_kind ADD VALUE IF NOT EXISTS 'installment'; -- one scheduled payment: principal + interest

-- general = everything so far; the other kinds are liabilities with extra behaviour.
ALTER TABLE accounts ADD COLUMN kind VARCHAR(20) NOT NULL DEFAULT 'general'
  CHECK (kind IN ('general', 'credit_card', 'paylater', 'loan'));

CREATE TABLE cards (
  account_id      UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  statement_day   SMALLINT NOT NULL CHECK (statement_day BETWEEN 1 AND 28),
  due_day         SMALLINT NOT NULL CHECK (due_day BETWEEN 1 AND 28),
  credit_limit    BIGINT NOT NULL DEFAULT 0 CHECK (credit_limit >= 0), -- sen, 0 = not set
  min_payment_pct SMALLINT NOT NULL DEFAULT 10 CHECK (min_payment_pct BETWEEN 0 AND 100)
);

CREATE TYPE installment_method AS ENUM ('flat', 'annuity');
CREATE TABLE installment_plans (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  account_id      UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE, -- the loan (liability)
  interest_account_id UUID NOT NULL REFERENCES accounts(id),                      -- an expense
  principal       BIGINT NOT NULL CHECK (principal > 0),        -- sen, the original loan
  annual_rate_bp  INT NOT NULL CHECK (annual_rate_bp BETWEEN 0 AND 10000), -- 1050 = 10.50% a year
  tenor_months    INT NOT NULL CHECK (tenor_months BETWEEN 1 AND 360),
  method          installment_method NOT NULL,
  first_due_date  DATE NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- A paid installment points at its journal entry; deleting the entry makes it unpaid again.
CREATE TABLE installment_payments (
  plan_id  UUID NOT NULL REFERENCES installment_plans(id) ON DELETE CASCADE,
  seq      INT NOT NULL,
  entry_id UUID NOT NULL UNIQUE REFERENCES journal_entries(id) ON DELETE CASCADE,
  PRIMARY KEY (plan_id, seq)
);

-- New default expense for interest and late fees; existing users get it unless 5108 is taken.
INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color)
SELECT id, '5108', 'Bunga & Denda', 'Expense', 'Beban', 'Debit', 'percent', 'red' FROM users
ON CONFLICT (user_id, code) DO NOTHING;
