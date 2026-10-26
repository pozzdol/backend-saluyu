UPDATE accounts SET kind = 'general' WHERE kind = 'saving';
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check
  CHECK (kind IN ('general', 'credit_card', 'paylater', 'loan'));
