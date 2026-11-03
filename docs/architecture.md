# Architecture

vergex is a single-user, self-hosted agent trading system for Binance and OKX USDT perpetuals.

- **Backend:** Go (module `vergex`), Gin, GORM with PostgreSQL, logrus, Starlark for Futures calculators
- **Frontend:** React 18, TypeScript, Vite, Tailwind v4, TanStack Query, lightweight-charts; pnpm
- **Deployment:** Docker Compose with local builds, driven by the Makefile

## Layout

```text
cmd/vergex/          entry point and local admin subcommands
internal/
  Infrastructure: knows nothing of trading
  config/ logger/
  service/           goroutines that survive a panic, and what each background service last reported
  security/          SSRF checks; auth/ for JWT; crypto/ for stored keys and transport
  database/          connection, migrations, serializers, core.settings; testdb/ for tests

  Shared domain: features depend on these, never the reverse
  user/              the user and their password
  agent/             model clients and their tables; provider/ for the registry, pricing/ for costs
  market/            candles and indicators
    feed/            live data hub, local candles, hydrator and stream writer
    instrument/      what can be traded, and naming a symbol
    ws/              one WebSocket connection to a changing set of topics
  venue/             one directory per venue, and nothing else
    binance/ okx/    account, orders, executor and fills (live order paths), and public data
  account/           accounts and their keys, what orders go through, exact decimals, the ledger
    syncer/          reads each active account's new trades into the ledger

  Features: each has a schema and a page, and none imports another
  trader/            the trader loop, risk control, position sync, and the trader's tables
    kind/            the kinds of trader: their config, limits and presets
    agent/           coin selection, prompts, decision parsing; the grid's prompts
    grid/            the grid's pure calculations
  futures/          plans and account closes
    agent/           chat, tools and the model adapter
    plan/            plan compiler, risk and reward
    run/             approved execution and reconciliation (live order path)
    forge/ plugins/  entry calculators, and public data the agent can ask for
  forecast/          experimental signals, their strategies, records and backtests

  Surfaces: read every feature, and nothing imports them
  dashboard/         the event feed and the overview
  assistant/         the skill-first assistant, whatever it is reached from
  telegram/          bot and alerts
  api/               HTTP API and JWT auth, one file per module
web/                 React frontend (see web/README.md)
docker/              Dockerfiles and the frontend nginx config
scripts/             key generation for make init; offline parameter search
```

## Requests

```text
Browser (React)                Telegram bot
  Dashboard · Forecast ·         skill-first agent
  Futures · Traders ·                │
  Settings                            │
          │                           │
          └──────── Gin API (/api) ───┘
                        │
              ┌─────────┴─────────┐
           Futures            Traders
          (futures/)         (trader/)
              └─────────┬─────────┘
                     Accounts
             venue/binance · venue/okx
                        │
            Binance / OKX USDT perpetuals
```

In the Docker setup, the frontend container's nginx serves the static files and proxies `/api/` to the backend, which is not published on the host. During development Vite does the same on port 8000.

## Main flows

- **Traders:** `internal/trader/auto_trader_loop.go` builds the context, `trader/agent` asks the model for decisions, and the trader executes them within the limits in its settings. See [traders.md](traders.md).
- **Futures:** chat → compile → approve → execute → reconcile under `/api/futures/*`. The agent can read data, draft plans and draw on the chart, but it cannot place orders. See [futures.md](futures.md).
- **Assistant:** Telegram and the web app share one agent, which exposes task-level skills only, and high-risk actions wait for a confirmation checked in Go. See [telegram.md](telegram.md) and [assistant.md](assistant.md).

Code conventions and the rules for live order paths are in [AGENTS.md](../AGENTS.md).
