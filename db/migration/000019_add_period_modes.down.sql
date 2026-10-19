ALTER TABLE users DROP COLUMN period_weekday;
ALTER TABLE users DROP COLUMN period_mode;
UPDATE users SET period_start_day = 28 WHERE period_start_day > 28;
ALTER TABLE users DROP CONSTRAINT users_period_start_day_check;
ALTER TABLE users ADD CONSTRAINT users_period_start_day_check CHECK (period_start_day BETWEEN 1 AND 28);
