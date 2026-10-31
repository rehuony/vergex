# Architecture

vergex is a single-user, self-hosted AI trading system for Binance and OKX USDT perpetuals.

- **Backend:** Go (module `vergex`), Gin, GORM with SQLite (CGO) or PostgreSQL, logrus, Starlark for Frontier calculators
- **Frontend:** React 18, TypeScript, Vite, Tailwind v4, SWR, lightweight-charts; pnpm
- **Deployment:** Docker Compose with local builds, driven by the Makefile

## Layout

```text
cmd/vergex/          entry point and local admin subcommands
internal/
  api/               HTTP API, JWT auth, rate limiting, Frontier endpoints
  manager/           trader lifecycle: load from the database, assemble, start, stop
  trader/            Autopilot loop, risk control, grid, position sync
    binance/ okx/    exchange adapters (live order paths)
  kernel/            strategy engine: coin selection, prompts, decision parsing; grid engine
  planning/          Frontier
    harness/         AI chat and tools
    core/            plan compiler, risk and reward
    live/            approved execution and reconciliation (live order path)
    market/ plugins/ forge/ model/
  market/            candles and indicators (CoinAnk, Binance fallback)
  provider/          public data: Binance rankings, OKX listing, CoinAnk
  llm/               model clients, provider registry, SSE streaming, usage callback
  pricing/           models.dev price cache and cost estimates
  store/             GORM models and storage
  telegram/          Telegram bot and skill-first agent
  auth/ crypto/ security/ safe/ config/ logger/
web/                 React frontend (see web/README.md)
docker/              Dockerfiles and the frontend nginx config
scripts/             key generation for make init; offline Autopilot parameter search
```

## Requests

```text
Browser (React)                Telegram bot
  Dashboard · Frontier ·         skill-first agent
  Autopilot · Settings                │
          │                           │
          └──────── Gin API (/api) ───┘
                        │
        ┌───────────────┼────────────────┐
   Frontier planning  Autopilot       Grid engine
   (planning/)        (trader/, kernel/)
        └───────────────┼────────────────┘
               Exchange adapters
               trader/binance · trader/okx
                        │
            Binance / OKX USDT perpetuals
```

In the Docker setup, the frontend container's nginx serves the static files and proxies `/api/` to the backend, which is not published on the host. During development Vite does the same on port 8000.

## Main flows

- **Autopilot:** `internal/trader/auto_trader_loop.go` builds the context, `kernel` asks the model for decisions, and the trader executes them within the strategy's limits. See [autopilot.md](autopilot.md).
- **Frontier:** chat → compile → approve → execute → reconcile under `/api/planning/*`. The AI can read data, draft plans and draw on the chart, but it cannot place orders. See [frontier.md](frontier.md).
- **Telegram:** the agent exposes task-level skills only, and high-risk actions wait for a confirmation checked in Go. See [telegram.md](telegram.md).

Code conventions and the rules for live order paths are in [AGENTS.md](../AGENTS.md).
