<p align="center">
  <img src="docs/assets/vergex-banner.svg" alt="vergex — Intelligence that trades. An open trading system you run yourself." width="100%"/>
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="README.zh-CN.md">中文</a>
</p>

vergex is a self-hosted AI trading system for crypto USDT perpetual futures on **Binance** and **OKX**. You bring your own model API key and exchange API key; nothing is routed through a paid gateway and no usage data leaves your machine.

- **Frontier** — work through a trade with AI on a live chart, compare entry plans (market, limit, ladder, TWAP) and approve each order yourself.
- **Traders** — run a strategy continuously: the trader reads the market, asks the model for decisions and places orders within the strategy's risk limits.
- **Grid trading** — grid strategies with market-regime checks.
- **Telegram** — manage traders and strategies, read positions, troubleshoot and receive alerts from a Telegram bot.
- **Forecast** — experimental coin signals from locally stored market data, with named strategies, backtests and every call checked afterwards. Nothing trades on them, and so far they have shown no edge.

## Run

Requirements: Docker with Compose, `make`, `openssl`.

```bash
git clone <your vergex repository> vergex
cd vergex
make up
```

`make up` creates `.env` from `.env.example` on first run, generates `JWT_SECRET`, `DATA_ENCRYPTION_KEY` and `RSA_PRIVATE_KEY` if they are missing, then builds and starts the containers. Open **http://127.0.0.1:8000**.

| Command | What it does |
| :------ | :----------- |
| `make init` | Create `.env` and fill missing keys (existing values are kept) |
| `make up` | `init`, then build and start |
| `make down` | Stop and remove the containers (the database volume and `data/` are kept) |
| `make restart` | Restart the containers |
| `make logs [SERVICE=vergex-backend]` | Follow logs |
| `make status` | Container status and health check |
| `make regen-keys` | Regenerate all keys — stored exchange and model keys become unreadable |

The UI and its `/api/` proxy listen on `VERGEX_BIND_ADDRESS:VERGEX_FRONTEND_PORT` (default `127.0.0.1:8000`). For remote access keep that binding and put a TLS reverse proxy with an extra authentication layer in front: see [docs/deployment.md](docs/deployment.md).

To update, pull your changes and run `make up` again. Keep `.env`, `data/` and the `vergex-postgres` volume: they hold the keys and the database. Stopping the service does not close positions; check the exchange after restarts.

### Development without Docker

Use the Go version in [go.mod](go.mod), Docker for PostgreSQL, Node.js 24 and pnpm (run `corepack enable` once; the version is pinned in `web/package.json`).

```bash
make init          # once, for the keys in .env
make run-backend   # starts PostgreSQL from .env, backend on :3000
make run-frontend  # frontend dev server on :8000, proxies /api to :3000
make dev           # run both at once
```

`make test` runs the Go tests, which need PostgreSQL (`make run-postgresql`), and the frontend tests; `make lint` runs golangci-lint and ESLint.

## First session

1. **Create the account.** The first registered user is the only user of the instance.
2. **Add an AI model** in Settings → AI models: DeepSeek, OpenAI, Claude, Qwen, Gemini, Grok, Kimi or MiniMax with your own API key, optionally with a custom base URL and model name.
3. **Add an exchange account** in Settings → Exchanges: Binance or OKX USDT perpetuals ([API key guide](docs/exchanges.md)). Bind the key to the server IP shown there.
4. **Open Frontier** to plan a trade, or create a strategy and a trader on the Strategies and Traders pages.

Frontier asks for approval on every order. A running trader places orders on its own; starting and stopping it are separate actions.

## Models, data and costs

Models are called directly with your key. Token usage of every call is stored locally and priced with the public [models.dev](https://models.dev) price list; Settings → AI costs shows the estimated cost per model. Models without a published price are shown as unpriced.

Market data comes from free public endpoints: live exchange streams for candles and prices, kept in a local history, exchange order books, and coin rankings (24h volume, gainers, losers, open-interest change) computed from Binance public data. For OKX traders, ranked coins are limited to symbols that OKX lists.

## How execution works

- **Reviewable orders.** Frontier shows account, sizing, entry and exit details before approval.
- **Account-aware sizing.** Frontier sizes from the selected leverage and available margin, with costs reserved; tick size, step size and venue limits apply.
- **Exchange-side exits.** Stop-loss and take-profit orders are placed as fills are observed. Entry and protection are not atomic.
- **Reconciliation.** Uncertain submissions are checked against the exchange instead of being resubmitted.
- **Local secrets.** Exchange and model keys are stored AES-encrypted with `DATA_ENCRYPTION_KEY`. Protect `.env`, the database and backups.

## Architecture

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
   (planning/)        (trader/, kernel/)
        └───────────────┼────────────────┘
               Exchange adapters
               trader/binance · trader/okx
                        │
            Binance / OKX USDT perpetuals

Models: llm/ provider clients     Storage: PostgreSQL (GORM)
```

The Go packages live under `internal/`. See [docs/architecture.md](docs/architecture.md) for the layout and [AGENTS.md](AGENTS.md) for development conventions.

## Documentation

| Guide | Contents |
| :---- | :------- |
| [Getting started](docs/getting-started.md) | Install, first session, backups, account recovery |
| [AI models](docs/models.md) | Providers, custom endpoints and costs |
| [Exchange API keys](docs/exchanges.md) | Binance and OKX setup |
| [Frontier](docs/frontier.md) | Planning, live execution and recovery |
| [Traders and strategies](docs/traders.md) | Strategies, prompts, risk limits and grid |
| [Telegram](docs/telegram.md) | Assistant setup, skills, alerts and diagnostics |
| [Market data](docs/market-data.md) | Local candles, rate limits and history |
| [Forecast](docs/forecast.md) | Experimental signals, strategies and backtests |
| [Remote access](docs/deployment.md) | Reverse proxy with TLS and authentication |
| [Troubleshooting](docs/troubleshooting.md) | Common problems and fixes |
| [Architecture](docs/architecture.md) | Code layout and main flows |

## License

[AGPL-3.0](LICENSE). vergex is a personal fork of NOFX (github.com/NoFxAiOS/nofx), maintained independently.

<sub>Automated trading involves substantial risk. AI-driven strategies are experimental and can lose money. Size positions appropriately, understand each venue, and never trade funds you cannot afford to lose. See [DISCLAIMER.md](DISCLAIMER.md).</sub>
