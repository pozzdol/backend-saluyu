CREATE TABLE goals (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name       VARCHAR(100) NOT NULL,
  target     BIGINT NOT NULL CHECK (target > 0),
  deadline   DATE,
  account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, -- the savings that hold it
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Entries dated on or before this day can no longer change.
ALTER TABLE users ADD COLUMN locked_until DATE;

-- Set by clients that may retry (the offline Quick Add queue): the same id posts once.
ALTER TABLE journal_entries ADD COLUMN client_id UUID;
CREATE UNIQUE INDEX journal_entries_client_idx ON journal_entries (user_id, client_id) WHERE client_id IS NOT NULL;

CREATE TABLE group_invite_links (
  token_hash BYTEA PRIMARY KEY,
  group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  used_by    UUID REFERENCES users(id) ON DELETE SET NULL,
  used_at    TIMESTAMPTZ
);
