CREATE TABLE trader.traders (
    id                    uuid PRIMARY KEY,
    user_id               uuid NOT NULL REFERENCES core.users (id) ON DELETE CASCADE,
    name                  text NOT NULL,
    kind                  text NOT NULL CHECK (kind IN ('agent', 'grid')),
    config                jsonb NOT NULL,
    agent_model_id        uuid REFERENCES agent.models (id) ON DELETE SET NULL,
    account_id            uuid REFERENCES account.accounts (id) ON DELETE RESTRICT,
    initial_balance       double precision NOT NULL DEFAULT 0,
    scan_interval_minutes integer NOT NULL DEFAULT 15,
    margin_mode           text NOT NULL DEFAULT 'cross' CHECK (margin_mode IN ('cross', 'isolated')),
    running               boolean NOT NULL DEFAULT false,
    halted_at             timestamptz,
    halted_reason         text NOT NULL DEFAULT '',
    halted_until          timestamptz,
    resumed_at            timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    archived_at           timestamptz,
    CHECK (archived_at IS NOT NULL OR (agent_model_id IS NOT NULL AND account_id IS NOT NULL))
);
CREATE INDEX traders_user ON trader.traders (user_id, created_at DESC) WHERE archived_at IS NULL;
COMMENT ON TABLE trader.traders IS 'Automatic traders. Deleting one archives it, so its cycles, equity and ledger entries stay.';
COMMENT ON COLUMN trader.traders.kind IS 'What runs it: agent asks a model for decisions on a set of coins; grid has a model manage grid levels on one symbol. Set when created and never changes.';
COMMENT ON COLUMN trader.traders.config IS 'What the kind decides: coins, market data, risk limits and prompts for agent; the grid for grid. Edited in place: a running agent trader picks a change up on its next cycle, and a running grid trader restarts with it. Kept when archived.';
COMMENT ON COLUMN trader.traders.agent_model_id IS 'What it trades with, as is account_id. A model a trader in use runs on cannot be deleted, since the CHECK refuses the NULL; archiving lets go of both.';
COMMENT ON COLUMN trader.traders.initial_balance IS 'What return and drawdown are measured against.';
COMMENT ON COLUMN trader.traders.halted_at IS 'When a hard stop tripped, so a restart or a change of settings cannot quietly start trading again. NULL while none has.';
COMMENT ON COLUMN trader.traders.halted_until IS 'When the halt ends by itself. NULL means only the user ends it.';
COMMENT ON COLUMN trader.traders.resumed_at IS 'When the user last ended a halt. Peak equity counts from here, so a resumed trader does not trip on the same peak.';
COMMENT ON COLUMN trader.traders.archived_at IS 'When it was deleted; NULL while it is in use.';

CREATE TABLE trader.cycles (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    trader_id         uuid NOT NULL REFERENCES trader.traders (id) ON DELETE CASCADE,
    seq               integer NOT NULL,
    conversation_id   uuid REFERENCES agent.conversations (id) ON DELETE SET NULL,
    candidate_symbols text[] NOT NULL DEFAULT '{}',
    execution_log     text[] NOT NULL DEFAULT '{}',
    account_state     jsonb,
    success           boolean NOT NULL DEFAULT false,
    error             text,
    agent_duration_ms integer NOT NULL DEFAULT 0,
    finished_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trader_id, seq)
);
CREATE INDEX cycles_trader ON trader.cycles (trader_id, id DESC);
CREATE INDEX cycles_finished ON trader.cycles (trader_id, finished_at);
CREATE INDEX cycles_conversation ON trader.cycles (conversation_id);
COMMENT ON TABLE trader.cycles IS 'One run of a trader''s loop. The prompts and the reply are in the conversation.';
COMMENT ON COLUMN trader.cycles.seq IS 'The trader''s cycle number, from 1.';
COMMENT ON COLUMN trader.cycles.conversation_id IS 'NULL when the model was not called, or the conversation could not be saved.';
COMMENT ON COLUMN trader.cycles.account_state IS 'The account as this cycle saw it; NULL when it stopped before reading it.';
COMMENT ON COLUMN trader.cycles.finished_at IS 'When the cycle ended, after its orders.';

CREATE TABLE trader.decisions (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cycle_id    bigint NOT NULL REFERENCES trader.cycles (id) ON DELETE CASCADE,
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
    order_id    bigint REFERENCES account.orders (id) ON DELETE SET NULL,
    decided_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (cycle_id, seq)
);
CREATE INDEX decisions_order ON trader.decisions (order_id) WHERE order_id IS NOT NULL;
COMMENT ON TABLE trader.decisions IS 'What the model decided in a cycle, in the order it was carried out, and how that went.';
COMMENT ON COLUMN trader.decisions.outcome IS 'blocked means a risk guard stopped it and nothing reached the exchange; hold is a decision to do nothing.';
COMMENT ON COLUMN trader.decisions.order_id IS 'The order it became, when one was placed.';

CREATE TABLE trader.equity_snapshots (
    trader_id      uuid NOT NULL REFERENCES trader.traders (id) ON DELETE CASCADE,
    taken_at       timestamptz NOT NULL,
    total_equity   double precision NOT NULL DEFAULT 0,
    unrealized_pnl double precision NOT NULL DEFAULT 0,
    PRIMARY KEY (trader_id, taken_at)
);
COMMENT ON TABLE trader.equity_snapshots IS 'The account''s equity at each cycle, for the equity curve, peak equity and drawdown.';

CREATE TABLE trader.position_peaks (
    trader_id    uuid NOT NULL REFERENCES trader.traders (id) ON DELETE CASCADE,
    symbol       text NOT NULL,
    side         text NOT NULL CHECK (side IN ('long', 'short')),
    peak_pnl_pct double precision NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trader_id, symbol, side)
);
COMMENT ON TABLE trader.position_peaks IS 'The best a held position has done, so the drawdown monitor still knows after a restart how much of it has been given back. Removed once the position is gone.';
COMMENT ON COLUMN trader.position_peaks.peak_pnl_pct IS 'On margin, in percent. Written only when it rises by a point, so it can trail the true peak by up to that.';
