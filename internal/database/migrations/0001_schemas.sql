-- One schema per module. Features depend on the shared modules, never the reverse:
-- core <- agent, account, trader, futures, telegram
-- market <- account, forecast
-- agent <- trader, futures, telegram
-- account <- trader, futures
CREATE SCHEMA IF NOT EXISTS agent;
CREATE SCHEMA IF NOT EXISTS market;
CREATE SCHEMA IF NOT EXISTS forecast;
CREATE SCHEMA IF NOT EXISTS account;
CREATE SCHEMA IF NOT EXISTS trader;
CREATE SCHEMA IF NOT EXISTS futures;
CREATE SCHEMA IF NOT EXISTS telegram;

-- Everything lives in a module's schema, so a table named without one is an error.
DROP SCHEMA IF EXISTS public;
