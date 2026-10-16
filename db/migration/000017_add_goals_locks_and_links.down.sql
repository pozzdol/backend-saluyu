DROP TABLE group_invite_links;
DROP INDEX journal_entries_client_idx;
ALTER TABLE journal_entries DROP COLUMN client_id;
ALTER TABLE users DROP COLUMN locked_until;
DROP TABLE goals;
