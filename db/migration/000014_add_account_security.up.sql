ALTER TABLE users
  ADD COLUMN email_verified_at   TIMESTAMPTZ,
  ADD COLUMN sessions_revoked_at TIMESTAMPTZ,  -- "log out everywhere": older access tokens stop working
  ADD COLUMN totp_secret         BYTEA,        -- AES-GCM encrypted; NULL = no 2FA set up
  ADD COLUMN totp_enabled_at     TIMESTAMPTZ,  -- NULL while setup is not confirmed with a code
  ADD COLUMN totp_last_step      BIGINT NOT NULL DEFAULT 0; -- a code works once (no replay)
-- Accounts made before verification existed count as verified.
UPDATE users SET email_verified_at = created_at;

-- A session is a chain of rotated refresh tokens that share session_id.
ALTER TABLE refresh_tokens
  ADD COLUMN session_id UUID NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN user_agent TEXT NOT NULL DEFAULT '',
  -- ended = revoked on purpose (logout, revoke). A revoked but not ended token that comes
  -- back was rotated, so someone copied it: that still ends every session.
  ADD COLUMN ended BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX refresh_tokens_session_idx ON refresh_tokens (session_id);

CREATE TABLE email_tokens (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose    VARCHAR(10) NOT NULL CHECK (purpose IN ('verify', 'reset')),
  token_hash BYTEA NOT NULL UNIQUE, -- SHA-256 of the link token
  expires_at TIMESTAMPTZ NOT NULL,
  used_at    TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE totp_backup_codes (
  id        BIGSERIAL PRIMARY KEY,
  user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash BYTEA NOT NULL, -- SHA-256; codes have 50 random bits
  used_at   TIMESTAMPTZ
);
CREATE INDEX totp_backup_codes_user_idx ON totp_backup_codes (user_id);
