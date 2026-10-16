-- Every user gets their own chart of accounts. Codes are unique per user, not globally.
ALTER TABLE accounts
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    ALTER COLUMN code DROP DEFAULT,
    DROP CONSTRAINT accounts_code_key;

-- Old global rows are dev data with no journal behind them.
DELETE FROM accounts WHERE user_id IS NULL;

-- Seed users that already exist. New users are seeded by Go at register time
-- (model.DefaultAccounts); this template must stay identical to that one.
INSERT INTO accounts (user_id, code, name, type, category, normal_balance)
SELECT u.id, t.code, t.name, t.type::account_type, t.category,
       CASE WHEN t.type IN ('Asset', 'Expense') THEN 'Debit' ELSE 'Credit' END::normal_balance
FROM users u CROSS JOIN (VALUES
    ('1001', 'Bank', 'Asset', 'Kas & Bank'),
    ('1002', 'Cash', 'Asset', 'Kas & Bank'),
    ('1003', 'Tabungan', 'Asset', 'Tabungan'),
    ('1004', 'E-wallet', 'Asset', 'Kas & Bank'),
    ('1005', 'Investasi', 'Asset', 'Investasi'),
    ('1101', 'Piutang', 'Asset', 'Piutang'),
    ('2101', 'Utang', 'Liability', 'Utang'),
    ('3001', 'Ekuitas Saldo Awal', 'Equity', 'Ekuitas'),
    ('4001', 'Gaji', 'Revenue', 'Pendapatan'),
    ('4002', 'Bunga', 'Revenue', 'Pendapatan'),
    ('4003', 'Pengembalian', 'Revenue', 'Pendapatan Lain'),
    ('4199', 'Pendapatan Lain-lain', 'Revenue', 'Pendapatan Lain'),
    ('5101', 'Transportasi', 'Expense', 'Beban'),
    ('5102', 'Makan', 'Expense', 'Beban'),
    ('5103', 'Jajan', 'Expense', 'Beban'),
    ('5104', 'Entertainment', 'Expense', 'Beban'),
    ('5105', 'Keluarga', 'Expense', 'Beban'),
    ('5107', 'Admin', 'Expense', 'Beban'),
    ('5199', 'Beban Tunai Lain-lain', 'Expense', 'Beban')
) AS t(code, name, type, category);

ALTER TABLE accounts
    ALTER COLUMN user_id SET NOT NULL,
    ADD CONSTRAINT accounts_user_code_key UNIQUE (user_id, code);
