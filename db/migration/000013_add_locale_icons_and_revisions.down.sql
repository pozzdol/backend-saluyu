DROP TABLE IF EXISTS loan_change_requests;
DROP TYPE IF EXISTS change_request_status;
DROP TABLE IF EXISTS entry_revisions;
ALTER TABLE accounts DROP COLUMN IF EXISTS icon, DROP COLUMN IF EXISTS color;
ALTER TABLE users DROP COLUMN IF EXISTS locale;
