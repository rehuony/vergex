ALTER TABLE trader.traders
    ADD COLUMN halted_at     timestamptz,
    ADD COLUMN halted_reason text NOT NULL DEFAULT '',
    ADD COLUMN halted_until  timestamptz,
    ADD COLUMN resumed_at    timestamptz;
COMMENT ON COLUMN trader.traders.halted_at IS 'When a hard stop tripped, so a restart or a change of settings cannot quietly start trading again. NULL while none has.';
COMMENT ON COLUMN trader.traders.halted_until IS 'When the halt ends by itself. NULL means only the user ends it.';
COMMENT ON COLUMN trader.traders.resumed_at IS 'When the user last ended a halt. Peak equity counts from here, so a resumed trader does not trip on the same peak.';
