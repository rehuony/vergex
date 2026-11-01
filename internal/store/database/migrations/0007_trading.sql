CREATE TABLE trading.accounts (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    venue      text NOT NULL CHECK (venue IN ('binance', 'okx')),
    name       text NOT NULL DEFAULT '',
    api_key    text NOT NULL DEFAULT '',
    secret_key text NOT NULL DEFAULT '',
    passphrase text NOT NULL DEFAULT '',
    identity   text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX accounts_user ON trading.accounts (user_id);
COMMENT ON TABLE trading.accounts IS 'Exchange accounts, with the API keys that reach them. An account does not assume spot or futures.';
COMMENT ON COLUMN trading.accounts.api_key IS 'Encrypted with DATA_ENCRYPTION_KEY, as are secret_key and passphrase.';
COMMENT ON COLUMN trading.accounts.identity IS 'Hash of the venue and plaintext API key, so one key cannot be connected twice.';

CREATE TABLE trading.account_leases (
    account_id  uuid PRIMARY KEY REFERENCES trading.accounts(id),
    holder      text NOT NULL CHECK (holder IN ('frontier')),
    holder_id   text NOT NULL,
    acquired_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (holder, holder_id)
);
COMMENT ON TABLE trading.account_leases IS 'Who has reserved an account. Automatic trading cannot start on a leased account, and a leased account cannot be deleted.';
COMMENT ON COLUMN trading.account_leases.holder_id IS 'What holds it within the holder, such as a Frontier run. Not a foreign key, so trading knows nothing of its features.';

CREATE TABLE trading.orders (
    id                     bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id             uuid NOT NULL REFERENCES trading.accounts(id),
    instrument_id          bigint NOT NULL REFERENCES market.instruments(id),
    source                 text NOT NULL CHECK (source IN ('trader', 'frontier', 'manual', 'external')),
    source_id              text,
    seq                    integer,
    role                   text NOT NULL CHECK (role IN ('entry', 'close', 'sl', 'tp', 'grid')),
    client_order_id        text,
    venue_client_id        text,
    exchange_order_id      text,
    triggered_order_id     text,
    parent_client_order_id text,
    side                   text NOT NULL CHECK (side IN ('buy', 'sell')),
    position_side          text CHECK (position_side IN ('long', 'short')),
    type                   text NOT NULL DEFAULT '',
    status                 text NOT NULL CHECK (status IN ('scheduled', 'submitting', 'NEW', 'PARTIALLY_FILLED', 'FILLED', 'CANCELED', 'EXPIRED', 'REJECTED', 'UNKNOWN')),
    quantity               numeric NOT NULL,
    price                  numeric,
    stop_price             numeric,
    filled_quantity        numeric NOT NULL DEFAULT 0,
    avg_fill_price         numeric,
    reduce_only            boolean NOT NULL DEFAULT false,
    post_only              boolean NOT NULL DEFAULT false,
    leverage               integer,
    margin_mode            text,
    error                  text,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    closed_at              timestamptz
);
CREATE UNIQUE INDEX orders_client_id ON trading.orders (account_id, client_order_id) WHERE client_order_id IS NOT NULL;
CREATE UNIQUE INDEX orders_exchange_id ON trading.orders (account_id, instrument_id, exchange_order_id) WHERE exchange_order_id IS NOT NULL;
CREATE UNIQUE INDEX orders_source_seq ON trading.orders (source, source_id, seq) WHERE seq IS NOT NULL;
CREATE INDEX orders_source ON trading.orders (source, source_id, created_at DESC);
CREATE INDEX orders_updated_at ON trading.orders (updated_at);
COMMENT ON TABLE trading.orders IS 'One row per real order. The features that place orders are named by source and source_id, not foreign keys, so a new one needs no change here.';
COMMENT ON COLUMN trading.orders.source IS 'external is an order seen only through its fills, with nothing here to claim it.';
COMMENT ON COLUMN trading.orders.seq IS 'Order within its source, where the order of its orders means something.';
COMMENT ON COLUMN trading.orders.triggered_order_id IS 'The order a conditional order became when it triggered, which is what its fills name.';
COMMENT ON COLUMN trading.orders.position_side IS 'NULL in one-way mode and on spot.';

CREATE TABLE trading.positions (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id     uuid NOT NULL REFERENCES trading.accounts(id),
    instrument_id  bigint NOT NULL REFERENCES market.instruments(id),
    source         text NOT NULL CHECK (source IN ('trader', 'frontier', 'manual', 'external')),
    source_id      text,
    side           text NOT NULL CHECK (side IN ('long', 'short')),
    status         text NOT NULL CHECK (status IN ('open', 'closed')),
    entry_quantity numeric NOT NULL DEFAULT 0,
    quantity       numeric NOT NULL DEFAULT 0,
    entry_price    numeric NOT NULL DEFAULT 0,
    exit_price     numeric NOT NULL DEFAULT 0,
    realized_pnl   numeric NOT NULL DEFAULT 0,
    fee            numeric NOT NULL DEFAULT 0,
    leverage       integer,
    margin_mode    text,
    opened_at      timestamptz NOT NULL,
    closed_at      timestamptz,
    close_reason   text,
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX positions_one_open ON trading.positions (account_id, instrument_id, source, source_id, side) NULLS NOT DISTINCT WHERE status = 'open';
CREATE INDEX positions_source ON trading.positions (source, source_id, status, closed_at DESC);
CREATE INDEX positions_updated_at ON trading.positions (updated_at);
COMMENT ON TABLE trading.positions IS 'Positions as worked out from fills, kept per source because an exchange only knows one per account.';
COMMENT ON COLUMN trading.positions.entry_quantity IS 'All that was ever opened; quantity is what is still open.';
COMMENT ON COLUMN trading.positions.fee IS 'Fees in dollar stablecoins. Others stay only on the fill.';

CREATE TABLE trading.fills (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id           bigint NOT NULL REFERENCES trading.orders(id),
    position_id        bigint REFERENCES trading.positions(id),
    account_id         uuid NOT NULL REFERENCES trading.accounts(id),
    instrument_id      bigint NOT NULL REFERENCES market.instruments(id),
    exchange_trade_id  text NOT NULL,
    side               text NOT NULL CHECK (side IN ('buy', 'sell')),
    price              numeric NOT NULL,
    quantity           numeric NOT NULL,
    quote_quantity     numeric NOT NULL,
    commission         numeric NOT NULL DEFAULT 0,
    commission_asset   text NOT NULL DEFAULT '',
    realized_pnl       numeric,
    unmatched_quantity numeric NOT NULL DEFAULT 0,
    is_maker           boolean NOT NULL DEFAULT false,
    executed_at        timestamptz NOT NULL,
    recorded_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, instrument_id, exchange_trade_id)
);
CREATE INDEX fills_account_time ON trading.fills (account_id, instrument_id, executed_at);
CREATE INDEX fills_order ON trading.fills (order_id);
CREATE INDEX fills_unmatched ON trading.fills (account_id, instrument_id, executed_at) WHERE unmatched_quantity > 0;
COMMENT ON TABLE trading.fills IS 'What actually traded, as the exchange reported it.';
COMMENT ON COLUMN trading.fills.realized_pnl IS 'As the exchange reported it. NULL when it did not, and on spot.';
COMMENT ON COLUMN trading.fills.position_id IS 'The first position this fill changed; a close can span several.';
COMMENT ON COLUMN trading.fills.unmatched_quantity IS 'Part of a close with no position to reduce yet. An open that arrives later and is older takes it.';

CREATE TABLE trading.skipped_fills (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id        uuid NOT NULL REFERENCES trading.accounts(id),
    native_symbol     text NOT NULL,
    exchange_trade_id text NOT NULL,
    detail            text NOT NULL,
    executed_at       timestamptz NOT NULL,
    recorded_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, native_symbol, exchange_trade_id)
);
COMMENT ON TABLE trading.skipped_fills IS 'Trades an exchange reported that cannot be booked, such as on a contract it no longer describes, so that one of them does not hold up the rest and is not lost in silence.';
COMMENT ON COLUMN trading.skipped_fills.detail IS 'What is known of the trade, and why it cannot be booked. Its numbers are as the exchange sent them.';
