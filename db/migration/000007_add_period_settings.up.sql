-- Payday cycle start (1..28: exists in every month, no clamping needed).
ALTER TABLE users ADD COLUMN period_start_day SMALLINT NOT NULL DEFAULT 25
    CHECK (period_start_day BETWEEN 1 AND 28);
-- Date used by period reports (dashboard, income statement). NULL = entry_date. Set when salary
-- arrives early for the next period; entry_date keeps the real bank date (balance sheet uses it).
ALTER TABLE journal_entries ADD COLUMN period_date DATE;
