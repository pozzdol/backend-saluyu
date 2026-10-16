-- The day the user's books open. NULL = not opened yet: only the opening entry may be posted,
-- and no entry may be dated before it (enforced in LedgerService.post).
ALTER TABLE users ADD COLUMN start_date DATE;

-- Users that already have entries keep them: their books open on their first entry date.
UPDATE users u SET start_date = (SELECT MIN(entry_date) FROM journal_entries e WHERE e.user_id = u.id);

-- At most one opening entry per user, guarded by the database rather than by code.
CREATE UNIQUE INDEX journal_entries_one_opening_idx ON journal_entries (user_id) WHERE kind = 'opening';
