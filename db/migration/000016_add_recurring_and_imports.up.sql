CREATE TYPE recurring_frequency AS ENUM ('daily', 'weekly', 'monthly', 'payday');
CREATE TYPE recurring_mode AS ENUM ('remind', 'auto');

CREATE TABLE recurring_rules (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name            VARCHAR(100) NOT NULL,
  kind            VARCHAR(10) NOT NULL CHECK (kind IN ('income', 'expense', 'transfer')),
  from_account_id UUID NOT NULL REFERENCES accounts(id),
  to_account_id   UUID NOT NULL REFERENCES accounts(id),
  amount          BIGINT NOT NULL CHECK (amount > 0),
  frequency       recurring_frequency NOT NULL,
  day             SMALLINT,              -- weekly: 0 (Sun)..6, monthly: 1..31 (clamped to month end)
  mode            recurring_mode NOT NULL DEFAULT 'remind',
  next_date       DATE NOT NULL,         -- the first occurrence not handled yet
  end_date        DATE,
  last_error      TEXT,                  -- why the last auto-post failed (it then waits as a reminder)
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX recurring_rules_due_idx ON recurring_rules (next_date);

-- One row per handled occurrence: the primary key is what makes posting idempotent.
CREATE TABLE recurring_occurrences (
  rule_id  UUID NOT NULL REFERENCES recurring_rules(id) ON DELETE CASCADE,
  due_date DATE NOT NULL,
  entry_id UUID REFERENCES journal_entries(id) ON DELETE SET NULL, -- NULL = skipped
  PRIMARY KEY (rule_id, due_date)
);

CREATE TABLE category_rules (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  keyword    VARCHAR(100) NOT NULL,       -- matched case-insensitively inside the description
  min_amount BIGINT,                      -- sen, optional range
  max_amount BIGINT,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX category_rules_user_idx ON category_rules (user_id);

-- Column mapping remembered per account (the bank or e-wallet the file came from).
CREATE TABLE import_profiles (
  account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
  mapping    JSONB NOT NULL
);

-- Every imported (or matched) statement row, by hash, so re-importing finds it.
CREATE TABLE imported_rows (
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  row_hash   BYTEA NOT NULL,
  entry_id   UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
  PRIMARY KEY (account_id, row_hash)
);
CREATE INDEX imported_rows_entry_idx ON imported_rows (entry_id);
