-- Envelope budgets: with budget_carry on, the unspent (or overspent) part of each period's
-- budget rolls into the next one. Accumulation starts in the period containing
-- budget_carry_since (the day the flag was switched on); earlier periods never count.
ALTER TABLE accounts ADD COLUMN budget_carry BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE accounts ADD COLUMN budget_carry_since DATE;

-- 50/30/20 group of an account; NULL = not counted. Keep the defaults in sync with
-- model.DefaultNeedGroup, which seeds new users.
ALTER TABLE accounts ADD COLUMN need_group TEXT CHECK (need_group IN ('needs', 'wants', 'savings'));
UPDATE accounts SET need_group = 'needs'   WHERE code IN ('5101', '5102', '5105', '5107');
UPDATE accounts SET need_group = 'wants'   WHERE code IN ('5103', '5104');
UPDATE accounts SET need_group = 'savings' WHERE code IN ('1003', '1005');
