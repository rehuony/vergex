CREATE TABLE strategy.strategies (
    id             uuid PRIMARY KEY,
    user_id        uuid REFERENCES core.users (id) ON DELETE CASCADE,
    domain         text NOT NULL CHECK (domain IN ('trading', 'forecast')),
    type           text NOT NULL CHECK (type IN ('agent', 'grid', 'signal')),
    name           text NOT NULL,
    description    text NOT NULL DEFAULT '',
    config         jsonb NOT NULL,
    engine_version integer,
    is_active      boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CHECK ((domain = 'forecast') = (user_id IS NULL))
);
CREATE INDEX strategies_user ON strategy.strategies (user_id);
CREATE UNIQUE INDEX strategies_one_active ON strategy.strategies (domain, user_id) NULLS NOT DISTINCT WHERE is_active;
CREATE UNIQUE INDEX strategies_forecast_name ON strategy.strategies (name) WHERE domain = 'forecast';
COMMENT ON TABLE strategy.strategies IS 'Saved strategies of every feature. Trading ones can be edited; forecast ones never change, so a different idea is a new row.';
COMMENT ON COLUMN strategy.strategies.user_id IS 'The owner of a trading strategy; empty for forecast ones, which belong to no account.';
COMMENT ON COLUMN strategy.strategies.engine_version IS 'The forecast engine version it was saved under.';
COMMENT ON COLUMN strategy.strategies.is_active IS 'For trading, the default for new traders. For forecast, the one in use.';

CREATE TABLE strategy.backtests (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    strategy_id uuid NOT NULL REFERENCES strategy.strategies (id) ON DELETE CASCADE,
    options     jsonb NOT NULL,
    status      text NOT NULL CHECK (status IN ('running', 'finished', 'failed')),
    done        integer NOT NULL DEFAULT 0,
    total       integer NOT NULL,
    report      jsonb,
    error       text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL,
    finished_at timestamptz
);
CREATE INDEX backtests_strategy ON strategy.backtests (strategy_id);
COMMENT ON TABLE strategy.backtests IS 'One backtest run. The report holds its results; per-signal rows are not kept.';
