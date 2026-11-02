CREATE TABLE market.instruments (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    venue         text NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('perpetual', 'spot')),
    symbol        text NOT NULL,
    native_symbol text,
    base_asset    text NOT NULL,
    quote_asset   text NOT NULL,
    settle_asset  text,
    status        text NOT NULL DEFAULT 'trading',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (venue, kind, symbol)
);
COMMENT ON TABLE market.instruments IS 'What can be traded: one row per venue, market kind and symbol, so spot and perpetual BTCUSDT are different instruments.';
COMMENT ON COLUMN market.instruments.symbol IS 'The normalized name, such as BTCUSDT.';
COMMENT ON COLUMN market.instruments.native_symbol IS 'The venue''s own name, such as BTC-USDT-SWAP.';
COMMENT ON COLUMN market.instruments.settle_asset IS 'What a derivative settles in; empty for spot.';

CREATE TABLE market.series (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    instrument_id  bigint NOT NULL REFERENCES market.instruments (id),
    timeframe      text NOT NULL,
    tier           smallint NOT NULL,
    status         text NOT NULL CHECK (status IN ('active', 'inactive')),
    priority       integer NOT NULL,
    next_due_at    timestamptz NOT NULL,
    first_open_at  timestamptz,
    last_closed_at timestamptz,
    fail_count     integer NOT NULL DEFAULT 0,
    last_error     text NOT NULL DEFAULT '',
    last_opened_at timestamptz,
    updated_at     timestamptz NOT NULL,
    UNIQUE (instrument_id, timeframe)
);
CREATE INDEX series_due ON market.series (status, priority, next_due_at);
COMMENT ON TABLE market.series IS 'One instrument at one timeframe, and how far it has been filled.';
COMMENT ON COLUMN market.series.tier IS '1 watchlist, 2 among the highest-volume coins, 3 opened by the user.';
COMMENT ON COLUMN market.series.last_closed_at IS 'The water mark: the open time of the newest closed candle that joins the data before it without an unexplained hole.';
COMMENT ON COLUMN market.series.last_opened_at IS 'When the user last asked for this series.';

CREATE TABLE market.candles (
    series_id        bigint NOT NULL REFERENCES market.series (id),
    open_at          timestamptz NOT NULL,
    open             double precision NOT NULL,
    high             double precision NOT NULL,
    low              double precision NOT NULL,
    close            double precision NOT NULL,
    volume           double precision NOT NULL,
    quote_volume     double precision NOT NULL,
    trades           bigint NOT NULL,
    taker_buy_volume double precision NOT NULL,
    taker_buy_quote  double precision NOT NULL,
    PRIMARY KEY (series_id, open_at)
);
COMMENT ON TABLE market.candles IS 'Closed candles. Under 15 minutes they are kept for 90 days; the rest are kept for good.';

CREATE TABLE market.candle_gaps (
    series_id bigint NOT NULL REFERENCES market.series (id),
    from_at   timestamptz NOT NULL,
    to_at     timestamptz NOT NULL,
    PRIMARY KEY (series_id, from_at)
);
COMMENT ON TABLE market.candle_gaps IS 'Ranges the exchange confirmed it has no candles for, so they are not requested again and do not count as missing data.';

CREATE TABLE market.derivative_metrics (
    instrument_id bigint NOT NULL REFERENCES market.instruments (id),
    kind          text NOT NULL CHECK (kind IN ('funding', 'open_interest', 'long_short')),
    observed_at   timestamptz NOT NULL,
    value         double precision NOT NULL,
    PRIMARY KEY (instrument_id, kind, observed_at)
);
COMMENT ON TABLE market.derivative_metrics IS 'Timed readings of funding, open interest value and the long/short ratio.';
