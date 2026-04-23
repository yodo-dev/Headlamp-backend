CREATE TABLE password_reset_otps (
  id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  email       TEXT        NOT NULL,
  otp_hash    TEXT        NOT NULL,
  reset_token UUID        UNIQUE,
  expires_at  TIMESTAMPTZ NOT NULL,
  verified_at TIMESTAMPTZ,
  used_at     TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_password_reset_otps_email ON password_reset_otps (email);
CREATE INDEX idx_password_reset_otps_reset_token ON password_reset_otps (reset_token) WHERE reset_token IS NOT NULL;
