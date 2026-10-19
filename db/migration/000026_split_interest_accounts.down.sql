-- Everything moves back to 5108, then the two accounts go (only the ones this migration made).
UPDATE journal_lines l SET account_id = o.id
FROM accounts n, accounts o
WHERE l.account_id = n.id AND n.code IN ('5111', '5112') AND n.name IN ('Bunga Pinjaman', 'Bunga & Denda Kartu')
  AND o.user_id = n.user_id AND o.code = '5108';
UPDATE installment_plans p SET interest_account_id = o.id
FROM accounts n, accounts o
WHERE n.id = p.interest_account_id AND n.code = '5111' AND n.name = 'Bunga Pinjaman'
  AND o.user_id = n.user_id AND o.code = '5108';
UPDATE accounts SET name = 'Bunga & Denda' WHERE code = '5108' AND name = 'Denda & Bunga Lain';
DELETE FROM accounts a WHERE a.code IN ('5111', '5112') AND a.name IN ('Bunga Pinjaman', 'Bunga & Denda Kartu')
  AND NOT EXISTS (SELECT 1 FROM journal_lines l WHERE l.account_id = a.id)
  AND NOT EXISTS (SELECT 1 FROM installment_plans p WHERE p.interest_account_id = a.id);
