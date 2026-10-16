DROP TABLE totp_backup_codes;
DROP TABLE email_tokens;
DROP INDEX refresh_tokens_session_idx;
ALTER TABLE refresh_tokens DROP COLUMN ended, DROP COLUMN user_agent, DROP COLUMN session_id;
ALTER TABLE users
  DROP COLUMN totp_last_step,
  DROP COLUMN totp_enabled_at,
  DROP COLUMN totp_secret,
  DROP COLUMN sessions_revoked_at,
  DROP COLUMN email_verified_at;
