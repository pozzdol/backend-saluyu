-- Which person a receivable (1101) or payable (2101) line belongs to. Required on those
-- accounts and forbidden elsewhere; LedgerService.validate enforces it because a CHECK
-- cannot look at accounts.code. RESTRICT keeps a contact with history from vanishing.
ALTER TABLE journal_lines ADD COLUMN contact_id UUID REFERENCES contacts(id) ON DELETE RESTRICT;
CREATE INDEX journal_lines_contact_idx ON journal_lines (contact_id) WHERE contact_id IS NOT NULL;
