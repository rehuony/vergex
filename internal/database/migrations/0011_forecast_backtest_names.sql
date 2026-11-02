ALTER TABLE forecast.backtests ADD COLUMN name text NOT NULL DEFAULT '';
COMMENT ON COLUMN forecast.backtests.name IS 'What the user calls the run. Empty shows its strategy''s name.';
