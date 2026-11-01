# Architecture

vergex is a single-user, self-hosted agent trading system for Binance and OKX USDT perpetuals.

- **Backend:** Go (module `vergex`), Gin, GORM with PostgreSQL, logrus, Starlark for Frontier calculators
- **Frontend:** React 18, TypeScript, Vite, Tailwind v4, SWR, lightweight-charts; pnpm
- **Deployment:** Docker Compose with local builds, driven by the Makefile

## Layout

```text
cmd/vergex/          entry point and local admin subcommands
internal/
  api/               HTTP API, JWT auth, rate limiting, Frontier endpoints
  trader/            Trader loop, risk control, grid, position sync
    agent/           strategy engine: coin selection, prompts, decision parsing; grid engine
    manager/         trader lifecycle: load from the database, assemble, start, stop
  exchange/          one directory per exchange, and nothing else
    binance/ okx/    account, orders and fills (live order paths), and public data
  trading/           what all exchanges share: interfaces, order types, exact decimals
    accountsync/     reads each active account's new trades into the ledger
  planning/          Frontier
    agent/           chat, tools and the model adapter
    core/            plan compiler, risk and reward
    live/            approved execution and reconciliation (live order path)
    market/ plugins/ forge/
  market/            candles and indicators for trading
    feed/            live data hub, local candles, hydrator and stream writer
    instrument/      what can be traded, and naming a symbol
    wsfeed/          one WebSocket connection to a changing set of topics
  agent/             model clients, provider registry, SSE streaming, usage callback
    pricing/         models.dev price cache and cost estimates
  store/             GORM models and storage
  telegram/          Telegram bot and skill-first agent
  security/          SSRF checks; auth/ for JWT; crypto/ for stored keys and transport
  safe/ config/ logger/
web/                 React frontend (see web/README.md)
docker/              Dockerfiles and the frontend nginx config
scripts/             key generation for make init; offline strategy parameter search
```

## Requests

```text
Browser (React)                Telegram bot
  Dashboard · Forecast ·         skill-first agent
  Frontier · Traders ·                │
  Strategies · Settings               │
          │                           │
          └──────── Gin API (/api) ───┘
                        │
        ┌───────────────┼────────────────┐
   Frontier planning  Traders         Grid engine
   (planning/)        (trader/)
        └───────────────┼────────────────┘
               Exchange adapters
               exchange/binance · exchange/okx
                        │
            Binance / OKX USDT perpetuals
```

In the Docker setup, the frontend container's nginx serves the static files and proxies `/api/` to the backend, which is not published on the host. During development Vite does the same on port 8000.

## Main flows

- **Traders:** `internal/trader/auto_trader_loop.go` builds the context, `trader/agent` asks the model for decisions, and the trader executes them within the strategy's limits. See [traders.md](traders.md).
- **Frontier:** chat → compile → approve → execute → reconcile under `/api/planning/*`. The agent can read data, draft plans and draw on the chart, but it cannot place orders. See [frontier.md](frontier.md).
- **Telegram:** the agent exposes task-level skills only, and high-risk actions wait for a confirmation checked in Go. See [telegram.md](telegram.md).

Code conventions and the rules for live order paths are in [AGENTS.md](../AGENTS.md).
