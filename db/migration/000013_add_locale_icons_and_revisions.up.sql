-- NULL = follow the browser (id-* gets Indonesian, anything else English).
ALTER TABLE users ADD COLUMN locale VARCHAR(5) CHECK (locale IN ('id', 'en'));

-- Icon is a lucide icon name, color a palette token; both optional (UI falls back by type).
ALTER TABLE accounts ADD COLUMN icon VARCHAR(40), ADD COLUMN color VARCHAR(20);
UPDATE accounts SET icon = t.icon, color = t.color FROM (VALUES
  ('1001','landmark','blue'), ('1002','wallet','green'), ('1003','piggy-bank','teal'), ('1004','smartphone','violet'),
  ('1005','trending-up','indigo'), ('1101','hand-coins','orange'), ('2101','handshake','amber'), ('3001','scale','slate'),
  ('4001','briefcase','green'), ('4002','percent','teal'), ('4003','rotate-ccw','sky'), ('4199','sparkles','yellow'),
  ('5101','car','sky'), ('5102','utensils','orange'), ('5103','cookie','pink'), ('5104','clapperboard','violet'),
  ('5105','house','rose'), ('5107','receipt','slate'), ('5199','coins','zinc')
) AS t(code, icon, color) WHERE accounts.code = t.code;

-- Every edit keeps the entry as it was and as it became.
CREATE TABLE entry_revisions (
  id         BIGSERIAL PRIMARY KEY,
  entry_id   UUID NOT NULL REFERENCES journal_entries(id) ON DELETE CASCADE,
  user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  before     JSONB NOT NULL, -- {entry_date, period_date, description, lines:[...]}
  after      JSONB NOT NULL,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX entry_revisions_entry_idx ON entry_revisions (entry_id, changed_at);

-- Editing a loan the other member already mirrored asks them first.
CREATE TYPE change_request_status AS ENUM ('pending', 'approved', 'rejected', 'cancelled');
CREATE TABLE loan_change_requests (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  loan_request_id UUID NOT NULL REFERENCES loan_requests(id) ON DELETE CASCADE,
  requested_by    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  proposed        JSONB NOT NULL, -- the requester's new entry: {entry_date, description, lines}
  status          change_request_status NOT NULL DEFAULT 'pending',
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  responded_at    TIMESTAMPTZ
);
-- At most one open change per loan; a new edit replaces (cancels) the pending one.
CREATE UNIQUE INDEX loan_change_requests_one_pending ON loan_change_requests (loan_request_id) WHERE status = 'pending';
