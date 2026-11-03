# Database

PostgreSQL is the only database. Its schema comes from the SQL files in `internal/database/migrations`, applied at startup and only forward. GORM reads and writes rows; it never creates or changes a table. Tables and the columns that need it carry a `COMMENT`, so a database tool shows what they mean.

## Schemas

Each module has a schema, and holds its own tables in its own Go package. `public` is dropped, so a table named without a schema is an error.

| Schema | Tables | Holds |
| :----- | :----- | :---- |
| `core` | `users`, `settings`, `schema_migrations` | The user, one row of settings per module, the cached model price list, and which migrations have run |
| `agent` | `models`, `conversations`, `messages`, `calls` | The models a user can call, everything said to and by them, and each request's tokens, cost and time |
| `market` | `instruments`, `series`, `candles`, `candle_gaps`, `derivatives` | What can be traded and its local market history |
| `account` | `accounts`, `leases`, `orders`, `fills`, `positions`, `skipped_fills` | Accounts on a venue and the ledger of real orders, whichever feature placed them |
| `trader` | `traders`, `cycles`, `decisions`, `equity_snapshots`, `position_peaks` | Automatic traders with their kind and config, what each cycle did, their equity, and the best each held position has done |
| `futures` | `plans`, `runs`, `run_orders`, `account_closes`, `calculators` | Futures plans, their execution state, manual closes, and the entry calculators written in its conversations |
| `forecast` | `strategies`, `backtests`, `signals`, `outcome_stats`, `calibrations`, `alert_states` | Forecast's saved strategies, their backtests, forecasts, how they turned out, and calibration |
| `telegram` | `bot`, `alert_marks`, `visitors` | The bot, what its alerts have already said and who it refused |

Features depend on the shared modules, never the reverse, and every foreign key points down this list:

```text
core     <- agent, account, trader, futures, telegram
market   <- account, forecast
agent    <- trader, futures, telegram
account  <- trader, futures
```

Features meet only by name. The ledger has no foreign key to a feature: an order, a position or a lease says what it belongs to with `source` and `source_id`, so a new feature that trades needs no change to `account`. A conversation and a model call say which feature they belong to with `feature`, and what the feature keeps about a conversation, such as a Futures session's account and symbol or a Telegram chat's id, is in its `context`. The web assistant has no tables of its own: its conversations are in `agent.conversations` with the rest.

vergex is used by one person, so `market`, `forecast`, `telegram` and `core.settings` belong to the instance, and everything else to the user through `user_id`.

## Conventions

| Topic | Rule |
| :---- | :--- |
| Names | Always `schema.table` in Go, with no reliance on `search_path`. Tables are plural, columns snake_case, and a concept has one name from column to Go field to JSON: a venue's own IDs are `venue_order_id` and `venue_trade_id`, a state is `status`, an order in a list is `seq` |
| Times | `timestamptz`, named `…_at`. Go code that works in Unix milliseconds keeps `int64` through the `unixms` serializer, where 0 is NULL |
| Keys | `id`: `uuid` for things people configure, `bigint` identity for high-volume rows, and the content hash for a plan or calculator. Tables with a natural key use it instead |
| Foreign keys | `<table>_id`, each with its `ON DELETE`: `CASCADE` for what belongs to the row, `SET NULL` for a record that outlives it, `RESTRICT` for what must stay, such as the ledger |
| Booleans | No `is_` prefix: `running`, `active`, `maker` |
| Money | `numeric` in `account`, read as exact decimals. `double precision` in `market`, `forecast`, `trader` and for estimated costs |
| Choices | `text` with a `CHECK`, so a new value takes one migration. Order statuses keep the venues' own words, such as `FILLED` |
| JSON | `jsonb` only for what is queried or changes shape. A plan's body is `text`, because approval binds to its exact bytes |
| Columns | A column is kept only if something reads it, or it records a fact about money or when a row came to be |

## Deleting a conversation

The features people chat with a model in are `agent.ChatFeatures`: Futures, the web assistant and Telegram. Their conversations are deleted in one place, `deleteConversations` in `internal/api/conversations.go`, whether from a list's menu, from the assistant or with the account they were on. Each takes all it held:

| What refers to it | On delete |
| :---------------- | :-------- |
| `agent.messages` | Deleted: what was said, with the context sent, the thinking and the tool calls |
| `agent.calls` | Deleted: its model calls, so Agent costs no longer count them |
| `futures.calculators` | Deleted: the entry calculators written in it |
| `futures.plans` | Deleted by `deleteConversations` when the plan never ran. One that ran loses only the link, and is deleted on its own once its runs have settled (`PlanStore.Delete`), with those runs and their orders; the account's ledger keeps what it sent |
| `trader.cycles` | Loses the link. A trader's conversations are not chats, and are never deleted this way |

The web assistant forgets what it held of one in memory as it goes, and a Telegram chat notices on its next message and starts a new one. A summary the assistant writes of a long chat is one of its model calls, so it goes with the chat too.

A trader opens a conversation for each cycle, and the cycle points at it. One whose cycle is never saved is deleted by `DecisionStore.ForgetConversations`: at once when saving the cycle fails, and for one a stopped process left, when traders load, before any cycle runs. Its model call stays without the link, since a trader's costs are counted by the trader through the call's `source_id`. A model test's calls belong to no conversation and stay.

A new table that refers to a conversation says what happens to it: `ON DELETE CASCADE` for what belongs to the conversation, `ON DELETE SET NULL` for a record that outlives it, and a step in `deleteConversations` for a rule that depends on more, as a plan's does. A new feature people chat in joins `agent.ChatFeatures`, and from then on it is listed and deleted with the rest.

## Adding a migration

Add the next numbered file to `internal/database/migrations`. Files run in name order, all in one transaction, under a lock, so two processes cannot migrate at once. A file that changed after it was applied stops startup, since the database no longer matches what it says, so change a table with a new file rather than by editing an old one.

The nine files there are the baseline: the schemas, then one file per schema in dependency order. Tests run against a template database rebuilt whenever these files change: see [AGENTS.md](../AGENTS.md).

### Starting from the baseline

The baseline was redone in November 2026, without carrying data over from the one before. Until the schema settles a change may do that again; once it settles, changes go in new files that bring existing data along. A database built from an older baseline refuses to start with `migration 0002_core.sql changed after it was applied`.

To rebuild it, back up anything you need with `make backup`, stop the backend, and recreate the database. It is empty after this, and the backend builds the schema when it next starts:

```bash
docker compose stop vergex-backend
docker exec vergex-postgres sh -c 'psql -U "$POSTGRES_USER" -d postgres -c "DROP DATABASE IF EXISTS $POSTGRES_DB WITH (FORCE)" -c "CREATE DATABASE $POSTGRES_DB"'
```

Then `make up`, or `make run-backend` from source, and register again. Positions and orders on the exchange are not touched, but the new ledger knows nothing of them.

## What grows

`agent.messages` is never cleaned up automatically, and neither are `trader.cycles` or the ledger: see [getting-started.md](getting-started.md) for how much to expect and how to check. Candles under 15 minutes are kept for 90 days and the rest for good, as [market-data.md](market-data.md) describes.
