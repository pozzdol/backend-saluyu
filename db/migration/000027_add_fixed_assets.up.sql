-- C22: fixed assets (own category, so they never count as spendable money) and the equity
-- account that absorbs manual revaluations.
INSERT INTO accounts (user_id, code, name, type, category, normal_balance, icon, color)
SELECT id, v.code, v.name, v.type::account_type, v.category, v.nb::normal_balance, v.icon, v.color FROM users,
(VALUES ('1301', 'Kendaraan', 'Asset', 'Aset Tetap', 'Debit', 'car', 'sky'),
        ('1302', 'Properti', 'Asset', 'Aset Tetap', 'Debit', 'house', 'emerald'),
        ('1303', 'Elektronik', 'Asset', 'Aset Tetap', 'Debit', 'tv', 'slate'),
        ('3002', 'Penyesuaian Nilai Aset', 'Equity', 'Ekuitas', 'Credit', 'scale', 'violet')) AS v(code, name, type, category, nb, icon, color)
ON CONFLICT (user_id, code) DO NOTHING;
