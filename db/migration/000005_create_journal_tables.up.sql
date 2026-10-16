CREATE TYPE entry_kind AS ENUM (
    'income', 'expense', 'transfer', 'loan', 'repayment', 'opening', 'cash_count', 'adjustment'
);

CREATE TABLE journal_entries (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Per-user sequence (1, 2, 3, ...), unlike the Excel "J1" that restarted every day.
    entry_no    BIGINT NOT NULL,
    entry_date  DATE NOT NULL,
    kind        entry_kind NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, entry_no)
);
CREATE INDEX journal_entries_user_date_idx ON journal_entries (user_id, entry_date DESC, entry_no DESC);

CREATE TABLE journal_lines (
    id          BIGSERIAL PRIMARY KEY, -- stable line order for running balances
    entry_id    UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
    account_id  UUID NOT NULL REFERENCES accounts(id),
    -- Money in sen (1/100 rupiah), never float. Exactly one side is non-zero.
    debit       BIGINT NOT NULL DEFAULT 0,
    credit      BIGINT NOT NULL DEFAULT 0,
    CHECK (debit >= 0 AND credit >= 0 AND (debit = 0) <> (credit = 0))
);
CREATE INDEX journal_lines_entry_idx ON journal_lines (entry_id);
CREATE INDEX journal_lines_account_idx ON journal_lines (account_id);
