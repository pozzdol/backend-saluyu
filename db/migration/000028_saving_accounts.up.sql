-- An asset can be marked as savings (kind 'saving'): money moved there counts as
-- "moved to savings" on the dashboard. Until now that was read from the group name
-- ("Tabungan", "Investasi"), so a group of the user's own, like "Dana Darurat", never counted.
ALTER TABLE accounts DROP CONSTRAINT accounts_kind_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_kind_check
  CHECK (kind IN ('general', 'saving', 'credit_card', 'paylater', 'loan'));
UPDATE accounts SET kind = 'saving' WHERE type = 'Asset' AND category IN ('Tabungan', 'Investasi');
