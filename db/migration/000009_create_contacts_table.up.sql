-- People money is lent to or borrowed from (Excel had them as expense accounts: "Bau", "Lia").
CREATE TABLE contacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,
    -- Phase 5: the app user whose book mirrors this contact. Column only for now.
    linked_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- "Bau" and "bau" are the same person, like users_email_lower_key.
CREATE UNIQUE INDEX contacts_user_name_lower_key ON contacts (user_id, lower(name));
