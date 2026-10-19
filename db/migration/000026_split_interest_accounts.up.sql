-- C23: 5108 "Bunga & Denda" mixed loan interest, card interest and everything else.
-- Two new expense accounts for every user; bank fees stay on 5107 Admin (it already is Biaya Bank).
INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color)
SELECT id, '5111', 'Bunga Pinjaman', 'Expense', 'Beban', 'Debit', 'percent', 'amber' FROM users
ON CONFLICT (user_id, code) DO NOTHING;
INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color)
SELECT id, '5112', 'Bunga & Denda Kartu', 'Expense', 'Beban', 'Debit', 'credit-card', 'red' FROM users
ON CONFLICT (user_id, code) DO NOTHING;

-- Installment entries: their 5108 lines are loan interest.
UPDATE journal_lines l SET account_id = n.id
FROM journal_entries e, accounts o, accounts n
WHERE l.entry_id = e.id AND o.id = l.account_id AND o.code = '5108'
  AND n.user_id = o.user_id AND n.code = '5111' AND n.name = 'Bunga Pinjaman'
  AND e.kind = 'installment';

-- Entries that touch a credit card or paylater: their 5108 lines are card interest or late fees.
UPDATE journal_lines l SET account_id = n.id
FROM journal_entries e, accounts o, accounts n
WHERE l.entry_id = e.id AND o.id = l.account_id AND o.code = '5108'
  AND n.user_id = o.user_id AND n.code = '5112' AND n.name = 'Bunga & Denda Kartu'
  AND e.kind <> 'installment'
  AND EXISTS (SELECT 1 FROM journal_lines cl JOIN accounts ca ON ca.id = cl.account_id
              WHERE cl.entry_id = e.id AND ca.kind IN ('credit_card', 'paylater'));

-- Plans that charged interest to 5108 now use Bunga Pinjaman for the installments still to pay.
UPDATE installment_plans p SET interest_account_id = n.id
FROM accounts o, accounts n
WHERE o.id = p.interest_account_id AND o.code = '5108'
  AND n.user_id = o.user_id AND n.code = '5111' AND n.name = 'Bunga Pinjaman';

-- What is left on 5108 is neither; rename it where the user never changed the name.
UPDATE accounts SET name = 'Denda & Bunga Lain' WHERE code = '5108' AND name = 'Bunga & Denda';
