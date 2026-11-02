# Database

PostgreSQL is the only database. Its schema comes from the SQL files in `internal/database/migrations`, applied at startup and only forward. GORM reads and writes rows; it never creates or changes a table. Tables and the columns that need it carry a `COMMENT`, so a database tool shows what they mean.

## Schemas

Each module has a schema, and holds its own tables in its own Go package. `public` is dropped, so a table named without a schema is an error.

| Schema | Tables | Holds |
| :----- | :----- | :---- |
| `core` | `users`, `settings`, `schema_migrations` | The user, one row of settings per module, the cached model price list, and which migrations have run |
| `agent` | `models`, `conversations`, `messages`, `calls` | The models a user can call, everything said to and by them, and each request's tokens, cost and time |
| `market` | `instruments`, `series`, `candles`, `candle_gaps`, `derivative_metrics` | What can be traded and its local market history |
| `account` | `accounts`, `leases`, `orders`, `fills`, `positions`, `skipped_fills` | Accounts on a venue and the ledger of real orders, whichever feature placed them |
| `trader` | `traders`, `cycles`, `decisions`, `equity_snapshots`, `position_peaks` | Automatic traders with their kind and config, what each cycle did, and the best each held position has done |
| `futures` | `plans`, `calculators`, `runs`, `run_orders`, `account_closes` | Futures plans, the entry calculators written in its conversations, and execution state |
| `forecast` | `strategies`, `backtests`, `signals`, `outcome_stats`, `calibrations`, `alert_states` | Forecast's saved strategies, their backtests, forecasts, how they turned out, and calibration |
| `telegram` | `bot`, `alert_marks`, `visitors` | The bot, what its alerts have already said and who it refused |

Features depend on the shared modules, never the reverse:

```text
core     <- agent, account, trader, futures, telegram
market   <- account, forecast
agent    <- trader, futures, telegram
account  <- trader, futures
```

The ledger has no foreign key to a feature. An order or position says where it came from with `source` and `source_id`, so a new feature that trades needs no change to `account`.

## Conventions

| Topic | Rule |
| :---- | :--- |
| Names | Always `schema.table` in Go, with no reliance on `search_path` |
| Times | `timestamptz`. Go code that works in Unix milliseconds keeps `int64` through the `unixms` serializer, where 0 is NULL |
| Keys | `uuid` for things people configure, `bigint` identity for high-volume rows. A plan keeps its content-derived ID |
| Money | `numeric` in `account`, read as exact decimals. `double precision` in `market`, `forecast` and `trader` |
| Choices | `text` with a `CHECK`, so a new value takes one migration |
| JSON | `jsonb` only for what is queried or changes shape. A plan's body is `text`, because approval binds to its exact bytes |

## Adding a migration

Add the next numbered file to `internal/database/migrations`. Files run in name order, all in one transaction, under a lock, so two processes cannot migrate at once. A file that changed after it was applied stops startup, since the database no longer matches what it says, so change a table with a new file rather than by editing an old one.

The nine files there are the baseline: the schemas, then one file per schema in dependency order. Tests run against a template database rebuilt whenever these files change: see [AGENTS.md](../AGENTS.md).

## What grows

`agent.messages` is never cleaned up automatically, and neither are `trader.cycles` or the ledger: see [getting-started.md](getting-started.md) for how much to expect and how to check. Candles under 15 minutes are kept for 90 days and the rest for good, as [market-data.md](market-data.md) describes.
