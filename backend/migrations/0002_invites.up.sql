-- Admin-invites-only onboarding: no open registration. An admin creates an
-- invite; the invitee redeems the token to set username + password.
CREATE TABLE invites (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text NOT NULL,
    role       text NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    token_hash text NOT NULL UNIQUE,               -- sha256 of the raw token
    invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_invites_open ON invites(lower(email)) WHERE accepted_at IS NULL;
