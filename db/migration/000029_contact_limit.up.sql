-- A lending limit per contact ("plafon"): how much may be out with them at once.
-- NULL means no limit; the budget page shows what is still owed against it.
ALTER TABLE contacts ADD COLUMN credit_limit BIGINT CHECK (credit_limit > 0);
