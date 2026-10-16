-- Postgres cannot drop enum values: 'borrowing' and 'installment' stay in entry_kind,
-- so entries of those kinds are removed first and the values simply go unused.
DELETE FROM journal_entries WHERE kind IN ('borrowing', 'installment');
DROP TABLE installment_payments;
DROP TABLE installment_plans;
DROP TYPE installment_method;
DROP TABLE cards;
ALTER TABLE accounts DROP COLUMN kind;
DELETE FROM accounts a WHERE code = '5108' AND name = 'Bunga & Denda'
  AND NOT EXISTS (SELECT 1 FROM journal_lines l WHERE l.account_id = a.id);
