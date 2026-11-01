-- One schema per module. Features depend on the shared modules, never the reverse:
-- core <- agent, strategy, trading, trader, frontier, telegram
-- market <- trading, forecast
-- strategy <- forecast, trader
-- agent <- trader, frontier, telegram
-- trading <- trader, frontier
CREATE SCHEMA IF NOT EXISTS agent;
CREATE SCHEMA IF NOT EXISTS market;
CREATE SCHEMA IF NOT EXISTS strategy;
CREATE SCHEMA IF NOT EXISTS forecast;
CREATE SCHEMA IF NOT EXISTS trading;
CREATE SCHEMA IF NOT EXISTS trader;
CREATE SCHEMA IF NOT EXISTS frontier;
CREATE SCHEMA IF NOT EXISTS telegram;

-- Everything lives in a module's schema, so a table named without one is an error.
DROP SCHEMA IF EXISTS public;
