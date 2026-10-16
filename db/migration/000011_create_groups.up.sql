-- Sharing groups: members see each other's summaries (never transactions), and loans
-- between members are mirrored into both books after approval.
CREATE TYPE group_member_status AS ENUM ('active', 'left');

CREATE TABLE groups (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    created_by  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE group_members (
    group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     group_member_status NOT NULL DEFAULT 'active',
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at    TIMESTAMPTZ,
    PRIMARY KEY (group_id, user_id) -- joining again after leaving updates the same row
);
CREATE INDEX group_members_user_idx ON group_members (user_id, status);

-- Invites are stored per email, registered or not, so the pending list looks the same
-- either way and cannot reveal which emails have an account.
CREATE TABLE group_invites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id    UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    email       VARCHAR(100) NOT NULL, -- lowercase (model.NormalizeEmail)
    invited_by  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, email)
);
CREATE INDEX group_invites_email_idx ON group_invites (email);

-- One contact per linked person, so an approved loan always finds the same contact.
CREATE UNIQUE INDEX contacts_user_linked_key ON contacts (user_id, linked_user_id) WHERE linked_user_id IS NOT NULL;
