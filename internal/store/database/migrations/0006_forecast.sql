CREATE TABLE forecast.signals (
    id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    strategy_id       uuid NOT NULL REFERENCES strategy.strategies (id) ON DELETE CASCADE,
    series_id         bigint NOT NULL REFERENCES market.series (id),
    open_at           timestamptz NOT NULL,
    close             double precision NOT NULL,
    prob_candles      double precision NOT NULL,
    prob_full         double precision NOT NULL,
    display_prob      double precision NOT NULL,
    calibrated        boolean NOT NULL,
    direction         text NOT NULL,
    confidence        smallint NOT NULL,
    expected_move_pct double precision NOT NULL,
    support           double precision NOT NULL,
    resistance        double precision NOT NULL,
    regime            text NOT NULL,
    horizon           smallint NOT NULL,
    target_at         timestamptz NOT NULL,
    status            text NOT NULL CHECK (status IN ('open', 'verified', 'void')),
    outcome           text CHECK (outcome IN ('hit', 'miss', 'neutral')),
    target_close      double precision,
    actual_move_pct   double precision,
    verified_at       timestamptz,
    independent       boolean NOT NULL DEFAULT false,
    UNIQUE (strategy_id, series_id, open_at)
);
CREATE INDEX signals_due ON forecast.signals (target_at) WHERE status = 'open';
CREATE INDEX signals_independent ON forecast.signals (strategy_id, series_id, open_at) WHERE independent;
COMMENT ON TABLE forecast.signals IS 'One forecast per strategy and closed candle, and later how it turned out.';
COMMENT ON COLUMN forecast.signals.prob_candles IS 'Probability of a rise from candles alone.';
COMMENT ON COLUMN forecast.signals.prob_full IS 'Probability of a rise with derivatives added.';
COMMENT ON COLUMN forecast.signals.display_prob IS 'What is shown: prob_full until calibrated, then the calibrated value.';
COMMENT ON COLUMN forecast.signals.target_at IS 'The open time of the candle whose close judges this forecast.';
COMMENT ON COLUMN forecast.signals.independent IS 'Its window does not overlap an earlier independent signal of the series, and price moved. Calibration and honest sample sizes use only these.';

CREATE TABLE forecast.outcome_stats (
    strategy_id       uuid NOT NULL REFERENCES strategy.strategies (id) ON DELETE CASCADE,
    series_id         bigint NOT NULL REFERENCES market.series (id),
    month             date NOT NULL,
    direction         text NOT NULL,
    regime            text NOT NULL,
    confidence_band   smallint NOT NULL,
    judged            bigint NOT NULL DEFAULT 0,
    hits              bigint NOT NULL DEFAULT 0,
    misses            bigint NOT NULL DEFAULT 0,
    moved             bigint NOT NULL DEFAULT 0,
    rose              bigint NOT NULL DEFAULT 0,
    move_sum          double precision NOT NULL DEFAULT 0,
    independent_moved bigint NOT NULL DEFAULT 0,
    independent_hits  bigint NOT NULL DEFAULT 0,
    independent_rose  bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (strategy_id, series_id, month, direction, regime, confidence_band)
);
COMMENT ON TABLE forecast.outcome_stats IS 'Judged signals added up, written in the same transaction as each judgement, for win rates by any of these dimensions.';
COMMENT ON COLUMN forecast.outcome_stats.month IS 'The first day of the month the signal''s candle opened in.';
COMMENT ON COLUMN forecast.outcome_stats.confidence_band IS 'Confidence in steps of 20: 0 is 0-19, 4 is 80-100.';
COMMENT ON COLUMN forecast.outcome_stats.moved IS 'Judged signals where price moved. rose / moved is the base rate a hit rate has to beat.';
COMMENT ON COLUMN forecast.outcome_stats.move_sum IS 'Sum of actual_move_pct.';

CREATE TABLE forecast.calibrations (
    strategy_id uuid NOT NULL REFERENCES strategy.strategies (id) ON DELETE CASCADE,
    timeframe   text NOT NULL,
    samples     integer NOT NULL,
    knots       jsonb NOT NULL,
    built_at    timestamptz NOT NULL,
    PRIMARY KEY (strategy_id, timeframe)
);
COMMENT ON TABLE forecast.calibrations IS 'A strategy''s probability curve for one timeframe.';

CREATE TABLE forecast.alert_states (
    series_id bigint PRIMARY KEY REFERENCES market.series (id),
    alerted   boolean NOT NULL
);
COMMENT ON TABLE forecast.alert_states IS 'That a series has alerted; it re-arms once the score comes back inside the threshold.';
