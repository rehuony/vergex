# vergex development guide

For people and coding agents working in this repository. Read the [Rules](#rules) before changing code; the rest is reference. User-facing behaviour is documented in [docs/](docs/README.md).

## Project

vergex is a personal, self-hosted AI trading system, forked from an upstream project (see the README's License section) and no longer synced with it.

- **Markets:** crypto only, on Binance USDT-M futures and OKX USDT perpetuals.
- **Models:** direct calls with your own API key to DeepSeek, OpenAI, Claude, Qwen, Gemini, Grok, Kimi or MiniMax, with an optional custom base URL. There is no paid gateway, paid data source, referral tagging or telemetry.
- **Users:** single user. The first account registered is the only one.
- **Features:** Traders (automated trading on a strategy), Frontier (AI-assisted planning of individual trades), grid trading, a Telegram assistant and Forecast (experimental signals, not connected to trading).
- **Deployment:** local builds only, with Docker Compose and the Makefile. Remote access adds nginx on the host: see [docs/deployment.md](docs/deployment.md).

## Commands

Run these after every change:

```bash
make run-postgresql   # the Go tests need PostgreSQL
go build ./... && go vet ./... && go test ./...
cd web && pnpm build && pnpm test && pnpm lint
```

A Go test that needs a database uses `testdb.Open(t)` or `storetest.New(t)`, which give it a schema of its own in the `<DB_NAME>_test` database of the PostgreSQL in `.env`, or on `VERGEX_TEST_DATABASE_URL`. It fails rather than skips when there is no database.

The Makefile is the entry point for everything else, and `make help` lists it. Targets come in pairs (`run-backend`/`run-frontend`, `build-*`, `test-*`, `lint-*`, `fmt-*`, `deps-*`), and `make dev`, `build`, `test` and `lint` cover both sides.

## Rules

### Live order paths

`internal/trader/binance`, `internal/trader/okx` and `internal/planning/live` handle real money. Only rename and decouple code there; do not change order logic. If a change is truly needed, add tests and verify it on a small account before calling it done. Exchange testnets are not supported yet and will be added once the features are complete.

### Compatibility

This is a clean rewrite with no old deployments or data to support, so the code keeps no upstream compatibility paths:

- New Binance and OKX orders use the `vergex` clientOrderId prefix. Lookups of persisted Frontier orders accept only that prefix; any other is a route mismatch.
- Position history and statistics query the current trader ID only and do not chain other traders' records.
- `NormalizeProductSchema` turns unknown or removed coin source types into `static`. Nothing migrates them at startup.
- PostgreSQL is the only database. It only migrates incrementally, with GORM AutoMigrate.
- Use `vergex` in all new code, copy, logs and storage keys. Upstream is named only in the README's License section, as AGPL requires of modified versions, and in the fork record ([docs/fork.md](docs/fork.md)).
- Environment variables use the `VERGEX_*` prefix: `VERGEX_BIND_ADDRESS` (default `127.0.0.1`), `VERGEX_FRONTEND_PORT` (default 8000), and `VERGEX_BACKEND_PORT` (default 3000, used only when the backend port mapping is enabled).

### Commits

- Use English Conventional Commits, one commit per work item.
- Never commit `.env`, `data/` or `.claude/`.

### Documentation

- Technical documentation, including this file, is English only. `docs/` has one page per topic, indexed in `docs/README.md`.
- The README exists in English (`README.md`) and Chinese (`README.zh-CN.md`); change both together. Other languages can return once multi-language support is in place.
- When a change affects behaviour, settings or commands, update the matching docs in the same work item. Docs describe what the code does today.

## Stack and runtime

- **Backend:** Go (module `vergex`), Gin, GORM with PostgreSQL, Starlark for Frontier calculators. Logging is logrus, always through `internal/logger`.
- **Frontend:** React 18, TypeScript, Vite, Tailwind v4, SWR, lightweight-charts, Vitest, ESLint with Prettier. The package manager is pnpm (`corepack enable`; the version is pinned in `web/package.json`).

| Where | Backend | Frontend |
| :---- | :------ | :------- |
| Docker Compose | Service, container and image `vergex-backend`. Listens on 3000 (`API_SERVER_PORT`) in the container; not published on the host by default | Service, container and image `vergex-frontend`. nginx on port 80 serves static files and proxies `/api/`; published on `VERGEX_BIND_ADDRESS:VERGEX_FRONTEND_PORT` (default `127.0.0.1:8000`) |
| Local development | `make run-backend` on 3000, on PostgreSQL from `make run-postgresql` | `make run-frontend` (Vite) on 8000, proxying `/api` to 3000 |

Both containers and `vergex-postgres` share the `vergex-network` network. `.env` configures the database, which runs in that one container for `make up` and from source alike. `make up` keeps it on the internal network; `make run-postgresql` also publishes it on `127.0.0.1` at `DB_PORT` for the backend and tests running from source.

## Repository layout

| Path | Contents |
| :--- | :------- |
| `cmd/vergex/` | Entry point: `main.go` is the startup flow, `cli.go` the local subcommands such as `vergex reset-password` and `vergex market-backfill` |
| `internal/` | All backend Go packages, private to this module under Go's `internal` rule |
| `web/` | Frontend. The Frontier chart is `web/src/components/planning/planning-chart/`, on the surface shared with Forecast in `web/src/components/market-chart/`; the page's components are in `web/src/components/planning/` |
| `docker/` | Dockerfiles for both images and the frontend container's `nginx.conf` |
| `scripts/` | `env-keys.sh` generates keys for `make init` and `make regen-keys`; `optimize/` is an offline strategy parameter search in Python |
| `docs/` | Documentation |

Packages under `internal/`:

| Package | Responsibility |
| :------ | :------------- |
| `api/` | All HTTP endpoints, JWT auth, rate limiting; `handler_planning*.go` serves Frontier |
| `manager/` | Trader lifecycle: load from the database, assemble an AutoTrader, start and stop |
| `trader/` | AutoTrader main loop (`auto_trader_*.go`), risk control, grid, position sync |
| `trader/binance`, `trader/okx` | Exchange adapters for trader orders and Frontier execution |
| `trader/execution`, `trader/types`, `trader/syncloop` | Execution order types, exchange interfaces, order sync loop |
| `kernel/` | Strategy engine: coin selection, market data, system and user prompts, parsing and validating AI decisions; grid engine |
| `market/` | Candles and indicators for trading; reads a `LiveSource`, which `marketdata` provides |
| `provider/binance` | Rankings from Binance public data (volume, price change, open-interest change), crypto perpetuals only; a request budget (`limiter.go`), candle and derivatives reads, and the market stream (`stream.go`) |
| `provider/okx` | Tradable OKX USDT perpetuals, used to filter candidates for OKX traders; candles and the candle and mark price streams |
| `provider/wsfeed` | One WebSocket connection to a changing set of topics, with what differs between exchanges behind a `Protocol` |
| `marketdata/` | Live data and local candles. `Hub` holds the latest from the streams, `Writer` stores finished candles, `subscriptions` decides what the streams carry, and the hydrator fills the rest within the request budget. Candles and derivatives live in `market_*` tables on their own connection pool |
| `forecast/` | Forecast: `Evaluate` (factors, regime, score), strategies, records, verification, calibration, alerts and backtests, in the market database |
| `llm/` | Model clients and provider registry, request building, SSE streaming, token usage callback |
| `pricing/` | models.dev price cache and cost estimates |
| `planning/` | Frontier: `harness` (AI chat and tools), `core` (plan compiler, risk and reward), `live` (execution after approval, reconciliation), `market` (market data and discovery), `forge` (Starlark entry calculators), `plugins` (public data such as funding rate and open interest), `model` (adapter over `llm`) |
| `store/` | GORM models and storage: users, models, exchanges, traders, strategies, decisions, positions, orders, grids, AI charges, Telegram config |
| `telegram/` | Telegram bot; `telegram/agent` is the skill-first assistant |
| `auth/`, `crypto/`, `security/`, `safe/` | JWT; AES storage and RSA transport encryption; SSRF checks; safe goroutine start |
| `config/`, `logger/` | `.env` and environment settings; logrus-based logging |
| `servicestate/` | What each background service last reported, for the Dashboard |
| `activity/` | The Dashboard's event feed: reads the database and turns new rows into AI, order, money and market events |

## Core flows

### Traders

Details are in [docs/traders.md](docs/traders.md). The entry point is `runCycle` in `internal/trader/auto_trader_loop.go`:

1. Check the running state, hot-reload the strategy, check risk pauses.
2. Build the trading context: account, positions, candidates. `kernel.StrategyEngine.GetCandidateCoins` produces candidates from `static`, `volume_top`, `gainers`, `losers`, `oi_top`, `oi_low` or the combined `mixed`. Rankings come from `provider/binance`; OKX traders filter them with `provider/okx`.
3. `kernel` fetches candles and indicators, builds the prompts, calls the model and parses decisions.
4. Execute opens and closes by priority, and record orders, positions, decisions and AI usage. Repeated AI failures enter safe mode, which opens no new positions; a drawdown monitor goroutine handles emergency closes.

### Frontier

Details are in [docs/frontier.md](docs/frontier.md).

| Endpoint (`/api/planning/`) | Role |
| :-------------------------- | :--- |
| `chat` | Harness. Its tools only read data, draft plans and draw; they cannot place orders |
| `compile` | Compile a plan: risk and reward, fees, sizing |
| `approve` | Lock `plan_hash` and `approval_hash` |
| `execute` | `planning/live` runs a finite, immutable order basket |
| `reconcile`, `check-status` | Reconciliation and recovery |
| `candles`, `feed` | Chart history and SSE updates |
| `results` | What finished plans realized, read from the exchange |

`web/src/components/planning/planning-chart/planning-chart.ts` draws plan levels and the AI's drawings (`horizontal_line`, `trend_line`, `rectangle`).

### Market data

Details are in [docs/market-data.md](docs/market-data.md). Binance's rate limits are per IP and shared with live trading, so live data comes from the streams, and REST through `binance.Limiter` only starts history, fills holes and answers trading when the stream cannot vouch for its data. Calculations and backtests read the local market tables and never make requests. Do not call the exchange from them.

Nothing downstream may hold a stream up: `wsfeed` and `Hub` drop rather than wait, and a lost candle leaves its series behind for the hydrator. Whatever trading reads must carry its age, and go to the exchange when it is too old rather than be used silently.

### Forecast

Details are in [docs/forecast.md](docs/forecast.md). `forecast.Evaluate` is pure and takes the data up to a candle, so live forecasts and backtests agree. Keep it that way: anything it reads must come through its arguments, and a change to what it computes raises `EngineVersion`. Saved strategies never change: a different idea is saved as a new one. Nothing in `trader/` or `planning/` reads forecasts: Frontier's chart can overlay one through `/api/forecast/overlay/:symbol`, assembled in `internal/api`, and its web code uses `api/forecast` and the shared chart surface, not `components/forecast`.

## Telegram assistant

`internal/telegram/bot.go` receives messages and binds sessions. `internal/telegram/agent` keeps one Agent per session and handles each session's messages in order.

The assistant is a trading operations tool, not a general chatbot. The design is 80% fixed skills and 20% dynamic planning: Go code fixes the flow of known, frequent tasks, and the model only understands intent, picks a skill, fills in missing details and writes the reply. User-facing behaviour is in [docs/telegram.md](docs/telegram.md).

`internal/telegram/alerts.go` pushes events to the bound chat and `notify.go` sends them. The watcher only reads, so it adds no exchange requests and never touches the trading loop.

### Skills

The model sees only these skills (`internal/telegram/agent/skills.go`):

| Skill | Actions | Notes |
| :---- | :------ | :---- |
| `trader_management` | create, update, delete, start, stop, query | Create auto-selects the model or exchange when only one is enabled |
| `strategy_management` | create, update, delete, activate, duplicate, query | Update sends only `config_patch`; the server merges it |
| `model_management` | query | Read-only; keys and endpoints change only in the web UI |
| `exchange_management` | query | Read-only, with account connection state |
| `account_query` | overview, balance, positions, position_history, trades, orders, open_orders, decisions, statistics, ai_costs | Read-only, per trader |
| `close_position` | | Checks first that the position exists |
| `diagnose` | trader_start, model, exchange, order_execution | Collects facts along fixed paths and gives hints (`diagnose.go`) |

Skills call the local REST API with the bot's JWT over fixed paths, so all server-side validation and risk limits apply. There is no generic `api_request` tool.

### Target resolution (`resolve.go`)

Targets are matched by id, then exact name (case-insensitive), then a unique name substring. Several matches return `ambiguous` with the candidates; no match returns `not_found`. It never guesses. All missing fields are listed at once in `missing_slots`, so the model asks for them in one go.

### Confirming high-risk actions (`confirm.go`)

These actions are held rather than run: starting, stopping or deleting a trader; updating a running trader; `close_position`; deleting a strategy; and strategy changes touching `risk_control`, `grid_config` or `strategy_type`. The tool result returns `needs_confirmation` and a change summary for the model to relay.

Go code decides whether to run it, not the model. The user's next message must be an explicit confirmation (`确认`, `confirm`, `yes` and so on); anything else cancels. A held action expires after 5 minutes.

### Adding skills

- One skill maps to one complete user task. Don't add a skill for a single endpoint.
- Each action defines its required fields, inference rules, confirmation conditions and failure result shape.
- Route write operations through the `hold()` confirmation gate where it applies. Credential changes (model keys, exchange keys, Telegram binding, passwords) stay out of the assistant.
- Cover new actions in `agent_test.go`: target resolution, the confirmation gate and the request body.

## Frontend conventions

### Structure and copy

- Components go in `web/src/components/<area>/`, pages in `web/src/pages/`. `web/README.md` lists the other directories.
- Keep a file beside its only user, and move it to `hooks/`, `utils/` or `constants/` only when several areas share it. Shared modules must not import from `components/`, `pages/` or `routes/`.
- File and directory names use kebab-case.
- Imports use `./` within a directory and `@/` otherwise, never `../`.
- Use named exports. Use a default export only where a framework requires one.
- UI copy goes through `web/src/i18n`, and translations stay in the code. The UI is fixed to English for now, with no language option in the account menu (`LANGUAGE_CHOICE_ENABLED` in `language-context.tsx`). Other languages return once the features are complete.
- New code must pass ESLint and Prettier.
- Tests live in `web/src/tests/`, mirroring the source path after `src/`: the test for `components/ui/button.tsx` is `tests/components/ui/button.test.tsx`. A test that covers several components stays whole, under the directory of the area it covers. Not every file needs a test.

### Pages and scrolling

- **Pages** use `components/common/page.tsx`. A list page shows no title or description, which the top bar makes redundant: section tabs and actions share one row, and the title is only read by assistive technology. A detail page passes `detail` for one row with the back link, name, state, context and actions. `leading` puts a search box at the start of the actions row. Content fills the width, with side gutters from `--page-gutter` to match the top bar.
- **Page scrollbars** are drawn by OverlayScrollbars (`utils/page-scroll.ts`) on platforms where native scrollbars take up width: they float and appear while scrolling. Platforms with floating native scrollbars are left alone. Because `html` is what scrolls, lock the page with `lockPageScroll()`, never by changing `overflow` on `body`.
- **Other scroll areas** use `components/ui/scroll-area.tsx` (`ScrollArea`), with the same floating scrollbar and no width taken from content. Put padding in `ScrollArea`'s own `className`; the library moves it to the scroll viewport.

### Panels

A functional area of a page is a `Panel` from `components/common/panel.tsx`, and its loading, empty and error states are a centred `PanelState`. A choice between a few options shown side by side is `Segmented` from `components/common/segmented.tsx`, tabs inside a page are `SectionTabs` from `components/common/section-tabs.tsx`, and a floating panel is `Popover` from `components/ui/popover.tsx`. The Dashboard stays read-only: actions belong on the page a row opens. Its scene moves only when a real event happens: nothing may animate without one.

### Dialogs

Use `components/common/dialog.tsx`, and never hand-write a `fixed inset-0` overlay. `Dialog` provides the overlay, title, close button, Esc, focus trap, page scroll lock, scrolling body and bottom button bar, and mounts on `body`. The file also exports `DialogSteps`, `DialogField`, `DialogNote` and the input style constants.

### Controls

- **Buttons** use `Button` from `components/ui/button.tsx`; for links and other elements use `asChild` or `buttonVariants`. Choose `variant` and `size` by meaning. A one-off style goes in `className` as Tailwind classes, which override defaults through `cn`. Don't style buttons in page CSS or define button style constants.
- **Global element defaults** for `button`, `input`, `select` and `textarea` belong in `@layer base`; rules outside a layer beat the Tailwind classes on components.
- **Selects** use `AppSelect` from `components/ui/select.tsx` (Radix Select, popover styled like the account menu). A Radix popover in a dialog closes only itself on Esc: it sets `defaultPrevented`, and `Dialog` then stays open.
- **Switches** use `Switch` from `components/ui/switch.tsx`, a button with `role="switch"`.
- **Search boxes** above long lists use `SearchField` from `components/common/search-field.tsx`, filtering as you type.
- **Number and symbol inputs** in setting rows use `NumberField`, `PeriodsField` and `SymbolsField` from `components/common/form-fields.tsx`, and `FieldGrid` lays rows out in two columns on wide pages.
- **Pointer cursor** is applied to clickable controls globally (`styles/index.css`, `@layer base`), so don't add `cursor-pointer` to each one.

### Settings

AI models and exchange accounts are configured only in the Settings dialog; they have no pages of their own. Elsewhere, open a section with `SettingsLink` or `useOpenSettings()` (`?settings=models`, `?settings=exchanges`). Read this data through `hooks/use-connections.ts`, which shares one SWR cache so saved changes appear at once.

Settings sections, and other forms laid out the same way, use `components/common/setting-row.tsx`:

- **`SettingRow`** for each field: a 160px name column and a 36px-tall control filling the rest, with dividers between rows and no helper text under fields.
- **`SettingActions`** ends a group that saves together. Buttons sit on the right. Validation and result messages go in a single line on the left of the bar (`messageTone` sets error, success or in progress, as with the AI model test result), never under an input, so the layout doesn't jump.
- **`SettingList`** for list sections (AI models, exchanges): the list scrolls inside the section, and the bar with the add button stays pinned at the bottom.
  - Each entry is a `SettingItem`: icon, name, a `SettingBadge` right after the name, and buttons on the right.
  - Opening one shows its form in the same section, with `SettingBack` at the top.
  - Loading, empty and error states use a centred `SettingState`.

### Motion

Durations and curves are tokens in `styles/brand.css`: `--dur-fade`, `--dur-state`, `--dur-panel`, `--ease-move` and `--ease-fade`. Tailwind's `transition-*` classes default to the fade values. Overlays and pages use `animate-menu-in`, `animate-overlay-in`, `animate-dialog-in` and `animate-page-in`; a menu also sets an `origin-*` class so it grows from what opened it. Hover colour changes apply at once (`hover:duration-0`).

### Colour and themes

- Use theme tokens (`vergex-brand`, `vergex-text`, `vergex-text-muted`, `vergex-success`, `vergex-danger` and so on) and the variables in `styles/brand.css` (`--surface`, `--canvas`, `--line`, `--brand-soft`, `--shadow-lg`). No inline hex colours and no `bg-white`; card backgrounds use `bg-[var(--surface)]`.
- Icons come from `lucide-react`. No emoji.
- Light, dark and follow-system themes are supported (`contexts/theme-context.tsx`, `<html data-theme>`). Dark values override those variables in `:root[data-theme='dark']` in `styles/brand.css`, so hard-coded colours won't switch. Anything coloured from JS, such as charts, must read the variables and redraw on `THEME_EVENT`.
