CREATE TYPE loan_request_status AS ENUM ('pending', 'approved', 'rejected');

-- A loan posted to a contact linked to another user asks that user to record the mirror entry.
CREATE TABLE loan_requests (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- UNIQUE: one request per entry. RESTRICT: an entry with a request cannot silently vanish;
    -- the app deletes pending/rejected requests itself and refuses once approved.
    from_entry_id UUID NOT NULL UNIQUE REFERENCES journal_entries(id) ON DELETE RESTRICT,
    from_user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    to_user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount        BIGINT NOT NULL CHECK (amount > 0), -- sen, size of the change between the two
    status        loan_request_status NOT NULL DEFAULT 'pending',
    to_entry_id   UUID UNIQUE REFERENCES journal_entries(id) ON DELETE RESTRICT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    responded_at  TIMESTAMPTZ,
    CHECK (from_user_id <> to_user_id),
    CHECK ((status = 'approved') = (to_entry_id IS NOT NULL))
);
CREATE INDEX loan_requests_pending_idx ON loan_requests (to_user_id) WHERE status = 'pending';
