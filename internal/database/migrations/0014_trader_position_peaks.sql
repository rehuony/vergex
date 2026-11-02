CREATE TABLE trader.position_peaks (
    trader_id    uuid NOT NULL REFERENCES trader.traders(id) ON DELETE CASCADE,
    symbol       text NOT NULL,
    side         text NOT NULL CHECK (side IN ('long', 'short')),
    peak_pnl_pct double precision NOT NULL,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (trader_id, symbol, side)
);
COMMENT ON TABLE trader.position_peaks IS 'The best a held position has done, so the drawdown monitor still knows after a restart how much of it has been given back. Removed once the position is gone.';
COMMENT ON COLUMN trader.position_peaks.peak_pnl_pct IS 'On margin, in percent. Written only when it rises by a point, so it can trail the true peak by up to that.';
