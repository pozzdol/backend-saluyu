CREATE TABLE budgets (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id   UUID NOT NULL REFERENCES accounts(id),
    period_start DATE NOT NULL,
    amount       BIGINT NOT NULL CHECK (amount >= 0), -- sen
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, account_id, period_start)
);
-- Serves DISTINCT ON "latest budget with period_start <= this period" (budgets inherit forward without copying).
CREATE INDEX budgets_lookup_idx ON budgets (user_id, account_id, period_start DESC);
