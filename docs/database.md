# Database

PostgreSQL is the only database. Its schema comes from the SQL files in `internal/store/database/migrations`, applied at startup and only forward. GORM reads and writes rows; it never creates or changes a table. Tables and the columns that need it carry a `COMMENT`, so a database tool shows what they mean.

## Schemas

Each module has a schema. `public` is dropped, so a table named without a schema is an error.

| Schema | Tables | Holds |
| :----- | :----- | :---- |
| `core` | `users`, `settings`, `schema_migrations` | The account, one row of settings per module, and which migrations have run |
| `agent` | `models`, `conversations`, `messages`, `calls` | The models a user can call, everything said to and by them, and each request's tokens, cost and time |
| `market` | `instruments`, `series`, `candles`, `candle_gaps`, `derivative_metrics` | What can be traded and its local market history |
| `strategy` | `strategies`, `backtests` | Saved strategies of every feature and their backtests |
| `forecast` | `signals`, `outcome_stats`, `calibrations`, `alert_states` | Forecasts, how they turned out, and calibration |
| `trading` | `accounts`, `account_leases`, `orders`, `fills`, `positions`, `skipped_fills` | Exchange accounts and the ledger of real orders, whichever feature placed them |
| `trader` | `traders`, `cycles`, `decisions`, `equity_snapshots` | Automatic traders and what each cycle did |
| `frontier` | `plans`, `runs`, `run_orders`, `account_closes` | Frontier's plans and execution state |
| `telegram` | `bot`, `alert_marks` | The bot and what its alerts have already said |

Features depend on the shared modules, never the reverse:

```text
core     <- agent, strategy, trading, trader, frontier, telegram
market   <- trading, forecast
strategy <- forecast, trader
agent    <- trader, frontier, telegram
trading  <- trader, frontier
```

The ledger has no foreign key to a feature. An order or position says where it came from with `source` and `source_id`, so a new feature that trades needs no change to `trading`.

## Conventions

| Topic | Rule |
| :---- | :--- |
| Names | Always `schema.table` in Go, with no reliance on `search_path` |
| Times | `timestamptz`. Go code that works in Unix milliseconds keeps `int64` through the `unixms` serializer, where 0 is NULL |
| Keys | `uuid` for things people configure, `bigint` identity for high-volume rows. A plan keeps its content-derived ID |
| Money | `numeric` in `trading`, read as exact decimals. `double precision` in `market`, `forecast` and `trader` |
| Choices | `text` with a `CHECK`, so a new value takes one migration |
| JSON | `jsonb` only for what is queried or changes shape. A plan's body is `text`, because approval binds to its exact bytes |

## Adding a migration

Add the next numbered file to `internal/store/database/migrations`. Files run in name order, all in one transaction, under a lock, so two processes cannot migrate at once. A file that changed after it was applied stops startup, since the database no longer matches what it says, so change a table with a new file rather than by editing an old one.

The first ten files are one baseline per schema, in dependency order. Tests run against a template database rebuilt whenever these files change: see [AGENTS.md](../AGENTS.md).

## What grows

`agent.messages` is never cleaned up automatically, and neither are `trader.cycles` or the ledger: see [getting-started.md](getting-started.md) for how much to expect and how to check. Candles under 15 minutes are kept for 90 days and the rest for good, as [market-data.md](market-data.md) describes.
