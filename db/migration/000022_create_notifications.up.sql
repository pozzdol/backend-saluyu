-- A user without a row has every reminder on (the defaults below).
CREATE TABLE notification_prefs (
  user_id    UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  due_bills  BOOLEAN NOT NULL DEFAULT TRUE,
  budget     BOOLEAN NOT NULL DEFAULT TRUE,
  weekly     BOOLEAN NOT NULL DEFAULT TRUE,
  inactive   BOOLEAN NOT NULL DEFAULT TRUE,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per reminder already sent; the unique key is what makes the job safe to re-run.
CREATE TABLE notification_log (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind    VARCHAR(20) NOT NULL,
  ref     TEXT NOT NULL, -- e.g. due:<rule id>:<date>:h3, budget80:<account id>:<period start>
  sent_on DATE NOT NULL,
  PRIMARY KEY (user_id, kind, ref)
);
