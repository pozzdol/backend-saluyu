-- Payday may be 29..31 now: a month without that day starts the period on its last day.
ALTER TABLE users DROP CONSTRAINT users_period_start_day_check;
ALTER TABLE users ADD CONSTRAINT users_period_start_day_check CHECK (period_start_day BETWEEN 1 AND 31);
-- payday: from period_start_day; last_workday: from the month's last Mon–Fri;
-- calendar_month: from the 1st; weekly: from period_weekday (0 = Sunday).
ALTER TABLE users ADD COLUMN period_mode VARCHAR(16) NOT NULL DEFAULT 'payday'
    CHECK (period_mode IN ('payday', 'last_workday', 'calendar_month', 'weekly'));
ALTER TABLE users ADD COLUMN period_weekday SMALLINT NOT NULL DEFAULT 1
    CHECK (period_weekday BETWEEN 0 AND 6);
