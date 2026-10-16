-- The default 5104 was the only English name in an Indonesian chart of accounts. Rename it
-- where the user never changed it; a name they chose stays.
UPDATE accounts SET name = 'Hiburan' WHERE code = '5104' AND name = 'Entertainment';
