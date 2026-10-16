-- Per-user rows cannot become global again without clashing codes.
DELETE FROM accounts;
ALTER TABLE accounts
    DROP CONSTRAINT accounts_user_code_key,
    DROP COLUMN user_id,
    ADD CONSTRAINT accounts_code_key UNIQUE (code);
