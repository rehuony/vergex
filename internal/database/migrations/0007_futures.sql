CREATE TABLE futures.plans (
    id              text PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    account_id      uuid REFERENCES account.accounts(id) ON DELETE SET NULL,
    venue           text NOT NULL,
    symbol          text NOT NULL,
    status          text NOT NULL CHECK (status IN ('draft', 'approved', 'cancelled', 'expired', 'completed')),
    origin          text NOT NULL DEFAULT 'manual' CHECK (origin IN ('assistant', 'manual')),
    conversation_id uuid REFERENCES agent.conversations(id) ON DELETE SET NULL,
    revises_plan_id text REFERENCES futures.plans (id),
    body            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plans_recent ON futures.plans (user_id, updated_at DESC);
COMMENT ON TABLE futures.plans IS 'Trade plans, drafted by the assistant or by hand. Approval is bound to the digest of body, which never changes.';
COMMENT ON COLUMN futures.plans.id IS 'Derived from the plan''s content.';
COMMENT ON COLUMN futures.plans.account_id IS 'NULL for a plan drafted without an account.';
COMMENT ON COLUMN futures.plans.body IS 'The plan as compiled, as text so its digest stays checkable byte for byte.';

CREATE TABLE futures.runs (
    id               uuid PRIMARY KEY,
    user_id          uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    plan_id          text NOT NULL REFERENCES futures.plans (id),
    account_id       uuid NOT NULL REFERENCES account.accounts(id) ON DELETE CASCADE,
    account_identity text NOT NULL,
    plan_hash        text NOT NULL,
    approval_hash    text NOT NULL,
    state            text NOT NULL CHECK (state IN ('approved', 'starting', 'running', 'stopping', 'position_open', 'needs_attention', 'completed', 'cancelled', 'expired')),
    error            text,
    approved_at      timestamptz NOT NULL,
    started_at       timestamptz,
    deadline         timestamptz,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX runs_account ON futures.runs (account_id, state);
CREATE INDEX runs_recent ON futures.runs (user_id, approved_at DESC);
CREATE INDEX runs_identity ON futures.runs (account_identity, state);
CREATE INDEX runs_updated ON futures.runs (updated_at);
COMMENT ON TABLE futures.runs IS 'One approved execution of a plan. An account with an unfinished run is held by its lease, and one with real orders by the ledger.';
COMMENT ON COLUMN futures.runs.account_identity IS 'Hash of the account''s venue and API key at approval. A changed key stops the run.';
COMMENT ON COLUMN futures.runs.updated_at IS 'When anything about the run last changed.';

CREATE TABLE futures.run_orders (
    run_id            uuid NOT NULL REFERENCES futures.runs(id) ON DELETE CASCADE,
    seq               integer NOT NULL,
    role              text NOT NULL CHECK (role IN ('entry', 'sl', 'tp')),
    symbol            text NOT NULL,
    side              text NOT NULL CHECK (side IN ('long', 'short')),
    type              text NOT NULL DEFAULT '',
    client_id         text NOT NULL,
    venue_client_id   text NOT NULL DEFAULT '',
    parent_client_id  text NOT NULL DEFAULT '',
    exchange_order_id text NOT NULL DEFAULT '',
    price             text NOT NULL DEFAULT '',
    quantity          text NOT NULL DEFAULT '',
    status            text NOT NULL CHECK (status IN ('scheduled', 'submitting', 'NEW', 'PARTIALLY_FILLED', 'FILLED', 'CANCELED', 'REJECTED', 'EXPIRED', 'TRIGGERED', 'UNKNOWN')),
    filled            text NOT NULL DEFAULT '0',
    average           text NOT NULL DEFAULT '',
    reduce_only       boolean NOT NULL DEFAULT false,
    post_only         boolean NOT NULL DEFAULT false,
    error             text NOT NULL DEFAULT '',
    updated_at        timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, seq)
);
CREATE INDEX run_orders_refused ON futures.run_orders (updated_at) WHERE exchange_order_id = '' AND error <> '';
COMMENT ON COLUMN futures.run_orders.updated_at IS 'When this row last changed. A run''s own time also moves when a restart stops it.';
COMMENT ON TABLE futures.run_orders IS 'What a run''s controller intends and what the exchange told it, written before each order is sent and only by the controller. The ledger in account.orders is kept from fills and has its own view of the same orders.';
COMMENT ON COLUMN futures.run_orders.seq IS 'Position in the run''s list, which has meaning: entries come first, in their approved order.';
COMMENT ON COLUMN futures.run_orders.side IS 'The direction of the plan, on its stops and targets too.';
COMMENT ON COLUMN futures.run_orders.price IS 'Exact decimals as approved and as reported, as are quantity, filled and average, so nothing is reformatted on the way back.';

CREATE TABLE futures.account_closes (
    id               uuid PRIMARY KEY,
    user_id          uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    account_id       uuid NOT NULL REFERENCES account.accounts(id),
    account_identity text NOT NULL,
    symbol           text NOT NULL,
    side             text NOT NULL CHECK (side IN ('long', 'short')),
    quantity         text NOT NULL,
    entry            text NOT NULL,
    state            text NOT NULL CHECK (state IN ('unconfirmed', 'confirmed')),
    order_state      jsonb NOT NULL,
    updated_at       timestamptz NOT NULL
);
CREATE INDEX account_closes_unconfirmed ON futures.account_closes (account_identity, symbol, side) WHERE state = 'unconfirmed';
COMMENT ON TABLE futures.account_closes IS 'Manual closes from the Futures account panel, saved before they are sent so a retry can never close twice.';
COMMENT ON COLUMN futures.account_closes.side IS 'The side of the position being closed.';
COMMENT ON COLUMN futures.account_closes.order_state IS 'The order as last known, exact decimals included.';

CREATE TABLE futures.calculators (
    hash            text PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES agent.conversations(id) ON DELETE CASCADE,
    body            text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX calculators_conversation ON futures.calculators (conversation_id, created_at DESC);
COMMENT ON TABLE futures.calculators IS 'Entry calculators the assistant wrote in a conversation. A version never changes: a new one has a new hash.';
COMMENT ON COLUMN futures.calculators.hash IS 'Derived from the conversation and the calculator''s definition.';
COMMENT ON COLUMN futures.calculators.body IS 'The calculator with its tests and their results, as text so its hash stays checkable.';
