CREATE TABLE trader.traders (
    id                    uuid PRIMARY KEY,
    user_id               uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    name                  text NOT NULL,
    agent_model_id        uuid REFERENCES agent.models(id),
    account_id            uuid REFERENCES trading.accounts(id),
    strategy_id           uuid REFERENCES strategy.strategies(id),
    initial_balance       numeric NOT NULL DEFAULT 0,
    scan_interval_minutes integer NOT NULL DEFAULT 15,
    margin_mode           text NOT NULL DEFAULT 'cross' CHECK (margin_mode IN ('cross', 'isolated')),
    is_running            boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    archived_at           timestamptz,
    CHECK (archived_at IS NOT NULL OR (agent_model_id IS NOT NULL AND account_id IS NOT NULL AND strategy_id IS NOT NULL))
);
CREATE INDEX traders_user ON trader.traders (user_id, created_at DESC) WHERE archived_at IS NULL;
COMMENT ON TABLE trader.traders IS 'Automatic traders. Deleting one archives it, so its cycles, equity and ledger entries stay.';
COMMENT ON COLUMN trader.traders.agent_model_id IS 'What it trades with, as are account_id and strategy_id. In use they cannot be deleted; archiving lets go of all three.';
COMMENT ON COLUMN trader.traders.initial_balance IS 'What return and drawdown are measured against.';
COMMENT ON COLUMN trader.traders.archived_at IS 'When it was deleted; NULL while it is in use.';

CREATE TABLE trader.cycles (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    trader_id         uuid NOT NULL REFERENCES trader.traders(id) ON DELETE CASCADE,
    cycle_number      integer NOT NULL,
    conversation_id   uuid REFERENCES agent.conversations(id) ON DELETE SET NULL,
    candidate_symbols text[] NOT NULL DEFAULT '{}',
    execution_log     text[] NOT NULL DEFAULT '{}',
    account_state     jsonb,
    success           boolean NOT NULL DEFAULT false,
    error             text,
    agent_duration_ms integer NOT NULL DEFAULT 0,
    finished_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cycles_trader ON trader.cycles (trader_id, id DESC);
CREATE INDEX cycles_finished ON trader.cycles (trader_id, finished_at);
COMMENT ON TABLE trader.cycles IS 'One run of a trader''s loop. The prompts and the reply are in the conversation.';
COMMENT ON COLUMN trader.cycles.conversation_id IS 'NULL when the model was not called, or the conversation could not be saved.';
COMMENT ON COLUMN trader.cycles.account_state IS 'The account as this cycle saw it; NULL when it stopped before reading it.';
COMMENT ON COLUMN trader.cycles.finished_at IS 'When the cycle ended, after its orders.';

CREATE TABLE trader.decisions (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cycle_id    bigint NOT NULL REFERENCES trader.cycles(id) ON DELETE CASCADE,
    seq         integer NOT NULL,
    action      text NOT NULL,
    symbol      text NOT NULL DEFAULT '',
    quantity    double precision NOT NULL DEFAULT 0,
    leverage    integer NOT NULL DEFAULT 0,
    price       double precision NOT NULL DEFAULT 0,
    stop_loss   double precision NOT NULL DEFAULT 0,
    take_profit double precision NOT NULL DEFAULT 0,
    confidence  integer NOT NULL DEFAULT 0,
    reasoning   text NOT NULL DEFAULT '',
    outcome     text NOT NULL CHECK (outcome IN ('executed', 'failed', 'blocked', 'hold')),
    error       text,
    order_id    bigint REFERENCES trading.orders(id) ON DELETE SET NULL,
    decided_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cycle_id, seq)
);
CREATE INDEX decisions_order ON trader.decisions (order_id) WHERE order_id IS NOT NULL;
COMMENT ON TABLE trader.decisions IS 'What the model decided in a cycle, in the order it was carried out, and how that went.';
COMMENT ON COLUMN trader.decisions.outcome IS 'blocked means a risk guard stopped it and nothing reached the exchange; hold is a decision to do nothing.';
COMMENT ON COLUMN trader.decisions.order_id IS 'The order it became, when one was placed.';

CREATE TABLE trader.equity_snapshots (
    trader_id         uuid NOT NULL REFERENCES trader.traders(id) ON DELETE CASCADE,
    taken_at          timestamptz NOT NULL,
    total_equity      double precision NOT NULL DEFAULT 0,
    balance           double precision NOT NULL DEFAULT 0,
    available_balance double precision NOT NULL DEFAULT 0,
    unrealized_pnl    double precision NOT NULL DEFAULT 0,
    position_count    integer NOT NULL DEFAULT 0,
    margin_used_pct   double precision NOT NULL DEFAULT 0,
    PRIMARY KEY (trader_id, taken_at)
);
COMMENT ON TABLE trader.equity_snapshots IS 'The account''s equity at each cycle, for the equity curve.';
