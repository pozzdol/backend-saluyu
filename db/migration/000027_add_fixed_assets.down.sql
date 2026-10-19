DELETE FROM accounts a WHERE a.code IN ('1301', '1302', '1303', '3002')
  AND a.name IN ('Kendaraan', 'Properti', 'Elektronik', 'Penyesuaian Nilai Aset')
  AND NOT EXISTS (SELECT 1 FROM journal_lines l WHERE l.account_id = a.id);
