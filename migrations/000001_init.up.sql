-- wallets, wager_transactions (idempotency), wallet_ledger_entries (append-only)

CREATE TABLE wallets (
    id            TEXT        PRIMARY KEY,
    player_id     TEXT        NOT NULL,
    currency      TEXT        NOT NULL,
    balance_cents BIGINT      NOT NULL,
    version       BIGINT      NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL,
    CONSTRAINT wallets_balance_non_negative CHECK (balance_cents >= 0),
    CONSTRAINT wallets_version_positive CHECK (version >= 1),
    CONSTRAINT wallets_currency_len CHECK (char_length(currency) = 3),
    CONSTRAINT wallets_player_currency_unique UNIQUE (player_id, currency)
);

CREATE TABLE wager_transactions (
    id                       TEXT        PRIMARY KEY,
    origin                   TEXT        NOT NULL,
    kind                     TEXT        NOT NULL,
    status                   TEXT        NOT NULL,
    wallet_id                TEXT        NOT NULL REFERENCES wallets (id),
    player_id                TEXT        NOT NULL,
    provider_id              TEXT,
    external_transaction_id  TEXT,
    idempotency_key          TEXT,
    payload_hash             TEXT,
    round_id                 TEXT,
    game_id                  TEXT,
    amount_cents             BIGINT      NOT NULL,
    currency                 TEXT        NOT NULL,
    reference_external_id    TEXT,
    reference_internal_id    TEXT,
    failure_code             TEXT,
    result_balance_minor     BIGINT,
    result_balance_currency  TEXT,
    created_at               TIMESTAMPTZ NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL,
    CONSTRAINT wager_origin_check CHECK (origin IN ('INTERNAL', 'EXTERNAL')),
    CONSTRAINT wager_kind_check CHECK (kind IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')),
    CONSTRAINT wager_status_check CHECK (status IN ('PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED')),
    CONSTRAINT wager_amount_non_negative CHECK (amount_cents >= 0),
    CONSTRAINT wager_currency_len CHECK (char_length(currency) = 3),
    CONSTRAINT wager_external_fields_check CHECK (
        (origin = 'EXTERNAL'
            AND provider_id IS NOT NULL
            AND external_transaction_id IS NOT NULL
            AND idempotency_key IS NOT NULL
            AND payload_hash IS NOT NULL
            AND kind <> 'OPENING')
        OR
        (origin = 'INTERNAL'
            AND provider_id IS NULL
            AND external_transaction_id IS NULL
            AND idempotency_key IS NULL
            AND payload_hash IS NULL
            AND kind = 'OPENING')
    )
);

-- Idempotency: one external transaction id per provider
CREATE UNIQUE INDEX wager_transactions_provider_external_uidx
    ON wager_transactions (provider_id, external_transaction_id)
    WHERE provider_id IS NOT NULL AND external_transaction_id IS NOT NULL;

CREATE UNIQUE INDEX wager_transactions_provider_idempotency_uidx
    ON wager_transactions (provider_id, idempotency_key)
    WHERE provider_id IS NOT NULL AND idempotency_key IS NOT NULL;

CREATE INDEX wager_transactions_wallet_id_idx ON wager_transactions (wallet_id);

CREATE TABLE wallet_ledger_entries (
    id                   TEXT        PRIMARY KEY,
    wallet_id            TEXT        NOT NULL REFERENCES wallets (id),
    transaction_id       TEXT        NOT NULL REFERENCES wager_transactions (id),
    direction            TEXT        NOT NULL,
    amount_cents         BIGINT      NOT NULL,
    currency             TEXT        NOT NULL,
    balance_before_cents BIGINT      NOT NULL,
    balance_after_cents  BIGINT      NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ledger_direction_check CHECK (direction IN ('DEBIT', 'CREDIT')),
    CONSTRAINT ledger_amount_positive CHECK (amount_cents > 0),
    CONSTRAINT ledger_balances_non_negative CHECK (
        balance_before_cents >= 0 AND balance_after_cents >= 0
    ),
    CONSTRAINT ledger_currency_len CHECK (char_length(currency) = 3),
    CONSTRAINT ledger_balance_math_check CHECK (
        (direction = 'DEBIT' AND balance_after_cents = balance_before_cents - amount_cents)
        OR
        (direction = 'CREDIT' AND balance_after_cents = balance_before_cents + amount_cents)
    ),
    CONSTRAINT ledger_wallet_transaction_unique UNIQUE (wallet_id, transaction_id)
);

CREATE INDEX wallet_ledger_entries_wallet_id_idx ON wallet_ledger_entries (wallet_id);

-- Append-only: reject UPDATE and DELETE at the database level
CREATE OR REPLACE FUNCTION prevent_ledger_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'wallet_ledger_entries is append-only: % not allowed', TG_OP
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

CREATE TRIGGER wallet_ledger_entries_immutable
    BEFORE UPDATE OR DELETE ON wallet_ledger_entries
    FOR EACH ROW
    EXECUTE PROCEDURE prevent_ledger_mutation();
