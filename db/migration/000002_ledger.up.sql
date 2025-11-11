CREATE TABLE ledgers (
    id UUID PRIMARY KEY,
    ref_type TEXT NOT NULL,            -- 'trade'
    ref_id   UUID NOT NULL,            -- trades.id
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY,
    ledger_id UUID NOT NULL REFERENCES ledgers(id),
    account_id UUID NOT NULL REFERENCES accounts(id),
    amount NUMERIC(20,8) NOT NULL,     -- positive or negative
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
