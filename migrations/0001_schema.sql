-- +goose Up
CREATE TABLE skus (
  id        BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  code      TEXT NOT NULL UNIQUE,
  name      TEXT NOT NULL,
  image_url TEXT NOT NULL
);

CREATE TABLE parties (
  id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name  TEXT NOT NULL,
  phone TEXT NOT NULL
);

CREATE TABLE jobwork_issues (
  id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  party_id     BIGINT  NOT NULL REFERENCES parties(id),
  sku_id       BIGINT  NOT NULL REFERENCES skus(id),
  qty          INTEGER NOT NULL CHECK (qty > 0),
  received_qty INTEGER NOT NULL DEFAULT 0,
  rate_paise   BIGINT  NOT NULL CHECK (rate_paise >= 0),
  issued_on    DATE    NOT NULL,
  CONSTRAINT received_within_issued CHECK (received_qty >= 0 AND received_qty <= qty)
);

CREATE TABLE jobwork_receipts (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  issue_id        BIGINT  NOT NULL REFERENCES jobwork_issues(id),
  qty             INTEGER NOT NULL CHECK (qty > 0),
  received_on     DATE    NOT NULL,
  idempotency_key TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT receipts_idempotency_key_uniq UNIQUE (idempotency_key)  -- NULLs are distinct
);
CREATE INDEX receipts_issue_idx ON jobwork_receipts (issue_id);

CREATE TABLE vouchers (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  type            TEXT    NOT NULL,
  source_id       BIGINT  NOT NULL,
  amount_paise    BIGINT  NOT NULL CHECK (amount_paise >= 0),
  status          TEXT    NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','pushed','failed')),
  attempts        INTEGER NOT NULL DEFAULT 0,
  last_error      TEXT,
  client_ref      TEXT    NOT NULL UNIQUE,
  external_ref    TEXT,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_until    TIMESTAMPTZ,
  claim_token     UUID,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT vouchers_source_uniq UNIQUE (type, source_id)
);
CREATE INDEX vouchers_due_idx    ON vouchers (next_attempt_at) WHERE status = 'pending';
CREATE INDEX vouchers_status_idx ON vouchers (status, created_at);

-- +goose Down
DROP TABLE vouchers;
DROP TABLE jobwork_receipts;
DROP TABLE jobwork_issues;
DROP TABLE parties;
DROP TABLE skus;
