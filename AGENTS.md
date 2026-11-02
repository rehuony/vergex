# vergex development guide

For people and coding agents working in this repository. Read the [Rules](#rules) before changing code; the rest is reference. User-facing behaviour is documented in [docs/](docs/README.md).

## Project

vergex is a personal, self-hosted agent trading system, forked from an upstream project (see the README's License section) and no longer synced with it.

- **Markets:** crypto only, on Binance USDT-M futures and OKX USDT perpetuals.
- **Models:** direct calls with your own API key to DeepSeek, OpenAI, Claude, Qwen, Gemini, Grok, Kimi or MiniMax, with an optional custom base URL. There is no paid gateway, paid data source, referral tagging or telemetry.
- **Users:** single user. The first account registered is the only one.
- **Features:** Auto trading (traders, shown as 交易员, each with its own settings; the tab reads Auto trading and 自动交易), Futures (agent-assisted planning of individual trades), grid trading, a Telegram assistant and Forecast (experimental signals, not connected to trading).
- **Deployment:** local builds only, with Docker Compose and the Makefile. Remote access adds nginx on the host: see [docs/deployment.md](docs/deployment.md).

## Commands

Run these after every change:

```bash
make run-postgresql   # the Go tests need PostgreSQL
go build ./... && go vet ./... && go test ./...
cd web && pnpm build && pnpm test && pnpm lint
```

A Go test that needs a database uses `testdb.Open(t)`, which gives it a database of its own, cloned from the template `<DB_NAME>_test` on the PostgreSQL in `.env`, or from the database in `VERGEX_TEST_DATABASE_URL`. The template is dropped and rebuilt when the migrations change, so its name must end in `_test` and any other is refused. Clones are dropped when the test ends. A test fails rather than skips when there is no database.

The Makefile is the entry point for everything else, and `make help` lists it. Targets come in pairs (`run-backend`/`run-frontend`, `build-*`, `test-*`, `lint-*`, `fmt-*`, `deps-*`), and `make dev`, `build`, `test` and `lint` cover both sides.

## Rules

### Live order paths

These handle real money. Only rename and decouple code there; do not change order logic. If a change is truly needed, add tests and verify it on a testnet account before calling it done, and on a small account before it trades real money. A testnet account is made in Settings ([docs/accounts.md](docs/accounts.md#testnet)); it proves the flow, not a real key's permissions.

| Path | Files | Live order path |
| :--- | :---- | :-------------- |
| `internal/venue/binance`, `internal/venue/okx` | `account.go`, `orders.go`, `executor.go`, `fills.go`: account, orders, reading fills | Yes |
| | `klines.go`, `stream.go`, `derivatives.go`, `markets.go`, `book.go`, `ranking.go`, `listing.go`, `limiter.go`: public data and the request budget | No |
| | `permissions.go`: what an API key may do, read only | No |
| `internal/futures/run` | All | Yes |
| `internal/account` | The contract orders go through: `Trading`, `Executor`, `Order`, exact decimals | Yes |
| `internal/account/syncer` | All | No: it places no order and only reads the exchange. But cooldowns and statistics rest on what it books, so a change to how it reads a venue needs tests and goes on the small-account list |

### Compatibility

This is a clean rewrite with no old deployments or data to support, so the code keeps no upstream compatibility paths:

- New Binance and OKX orders use the `vergex` clientOrderId prefix. Lookups of persisted Futures orders accept only that prefix; any other is a route mismatch.
- Position history and statistics query the current trader ID only and do not chain other traders' records.
- A trader's config is decoded by its kind in `internal/trader/kind`, and a value outside the kind's ranges or choices is refused rather than guessed at.
- PostgreSQL is the only database. Its schema comes only from the forward-only SQL migrations in `internal/database/migrations`, applied at startup; GORM only reads and writes. To change a table, add a new file; a file changed after it was applied stops startup. See [docs/database.md](docs/database.md).
- Use `vergex` in all new code, copy, logs and storage keys. Upstream is named only in the README's License section, as AGPL requires of modified versions, and in the fork record ([docs/fork.md](docs/fork.md)).
- Environment variables use the `VERGEX_*` prefix: `VERGEX_BIND_ADDRESS` (default `127.0.0.1`), `VERGEX_FRONTEND_PORT` (default 8000), and `VERGEX_BACKEND_PORT` (default 3000, used only when the backend port mapping is enabled).

### Commits

- Use English Conventional Commits, one commit per work item.
- Never commit `.env` or `.claude/`.

### Documentation

- Technical documentation, including this file, is English only. `docs/` has one page per topic, indexed in `docs/README.md`.
- The README exists in English (`README.md`) and Chinese (`README.zh-CN.md`); change both together.
- When a change affects behaviour, settings or commands, update the matching docs in the same work item. Docs describe what the code does today.
- `DESIGN.md` at the root is the design file for the web interface, written to the template in the global design guide: identity, tokens, shape, elevation, components and the do and don't. Read it before changing anything the user sees, and change it in the same work item as the tokens in `web/src/styles`.

## Stack and runtime

- **Backend:** Go (module `vergex`), Gin, GORM with PostgreSQL, Starlark for Futures calculators. Logging is logrus, always through `internal/logger`.
- **Frontend:** React 18, TypeScript, Vite, Tailwind v4, TanStack Query, lightweight-charts, three.js (the Dashboard's stage, in a chunk only that page loads), qrcode.react (Telegram's binding step), Vitest, ESLint with Prettier. The package manager is pnpm (`corepack enable`; the version is pinned in `web/package.json`).

| Where | Backend | Frontend |
| :---- | :------ | :------- |
| Docker Compose | Service, container and image `vergex-backend`. Listens on 3000 (`API_SERVER_PORT`) in the container; not published on the host by default | Service, container and image `vergex-frontend`. nginx on port 80 serves static files and proxies `/api/`; published on `VERGEX_BIND_ADDRESS:VERGEX_FRONTEND_PORT` (default `127.0.0.1:8000`) |
| Local development | `make run-backend` on 3000, on PostgreSQL from `make run-postgresql` | `make run-frontend` (Vite) on 8000, proxying `/api` to 3000 |

Both containers and `vergex-postgres` share the `vergex-network` network. `.env` configures the database, which runs in that one container for `make up` and from source alike. `make up` keeps it on the internal network; `make run-postgresql` also publishes it on `127.0.0.1` at `DB_PORT` for the backend and tests running from source.

## Repository layout

| Path | Contents |
| :--- | :------- |
| `cmd/vergex/` | Entry point: `main.go` is the startup flow and joins the modules together, `cli.go` the local subcommands such as `vergex reset-password`, `vergex reset-user`, `vergex market-backfill` and `vergex prune` |
| `internal/` | All backend Go packages, private to this module under Go's `internal` rule |
| `web/` | Frontend. The Futures chart is `web/src/components/futures/futures-chart/`, on the surface shared with Forecast in `web/src/components/market-chart/`; the page's components are in `web/src/components/futures/` |
| `docker/` | Dockerfiles for both images and the frontend container's `nginx.conf` |
| `scripts/` | `env-keys.sh` generates keys for `make init` and `make regen-keys`; `optimize/` is an offline parameter search in Python |
| `docs/` | Documentation |
| `DESIGN.md` | How the web interface looks: identity, tokens, shape, components and the do and don't |

Packages under `internal/`, in layers. Each depends only on those above it, and each module owns its tables, with their models and queries beside the rest of its code. `internal/database` is the only persistence foundation.

| Layer | Package | Responsibility |
| :---- | :------ | :------------- |
| Infrastructure | `config/`, `logger/` | `.env` and environment settings; logrus-based logging |
| | `service/` | `service.Go` starts a goroutine that survives a panic; each background service reports here how it is doing, for the Dashboard; `service.VenueTransport` carries every REST request to a venue and drops connections that went silent |
| | `security/`, `security/auth`, `security/crypto` | SSRF checks; JWT; AES storage and RSA transport encryption |
| | `database/` | Opens PostgreSQL, holds the SQL migrations, serializers and `core.settings`; `database/testdb` gives tests a database |
| Shared domain | `user/` | The user and their password (`core.users`) |
| | `agent/`, `agent/provider`, `agent/pricing` | Model clients and provider registry, request building, streaming replies, token usage callback; the `agent.*` tables; models.dev price cache and cost estimates |
| | `market/` | Candles and indicators for trading; reads a `LiveSource`, which `market/feed` provides; `market.Public` is the public market data Futures reads |
| | `market/instrument` | What can be traded: venue, market kind and symbol, `Normalize` for naming a symbol, and order books. Market data and the ledger both refer to it |
| | `market/ws` | One WebSocket connection to a changing set of topics, with what differs between venues behind a `Protocol` |
| | `market/feed` | Live data and local candles. `Hub` holds the latest from the streams, `Writer` stores finished candles, `subscriptions` decides what the streams carry, and the hydrator fills the rest within the request budget. Candles and derivatives live in the `market` schema on their own connection pool |
| | `venue/binance`, `venue/okx` | One directory per venue and nothing else in `venue/`, not even shared files, so a new venue adds one directory, with matching file names. Each holds its adapter for trader orders and Futures execution, which hold keys, and its public data. Binance: rankings from public data (volume, price change, open-interest change), crypto perpetuals only; a request budget (`limiter.go`), candle and derivatives reads, and the market stream (`stream.go`). OKX: tradable USDT perpetuals, used to filter candidates for OKX traders; candles and the candle and mark price streams |
| | `account/` | What all venues share: the `Trading` interface automatic trading uses, the `Executor` contract Futures uses, order types and exact decimals. Also accounts and their keys, leases, and the ledger (`account.*`: orders, fills, positions). The ledger knows no feature: it says where an order came from with `source` and `source_id` |
| | `account/syncer` | Reads each active account's new trades into the ledger. What it asks of the features comes through `syncer.Features`, joined in `cmd/vergex/main.go` |
| Features | `trader/` | Lifecycle (`manager.go`: load from the database, assemble an AutoTrader, start and stop), the AutoTrader main loop (`auto_trader_*.go`), risk control, position sync, and the `trader.*` tables |
| | `trader/kind` | The kinds of trader, one file each: the config, `Validate`, `Limits`, `RiskFields`, presets; `Decode` and `Merge` by kind, and `EstimateTokens` |
| | `trader/agent` | Coin selection, market data, system and user prompts, parsing and validating agent decisions; the grid's prompts |
| | `trader/grid` | The grid's pure calculations: regime, box, levels |
| | `futures/` | Plans and account closes, and `futures.Market`. The feature, shown as Futures and 合约交易, not go-binance's `futures` package, which only `venue/binance` imports. `agent` (chat, tools and the model adapter), `plan` (plan compiler, risk and reward), `run` (execution after approval, reconciliation), `forge` (Starlark entry calculators), `plugins` (public data such as funding rate and open interest) |
| | `forecast/` | Forecast: `Evaluate` (factors, regime, score), strategies, records, verification, calibration, alerts and backtests, in the `forecast` schema |
| Surfaces | `dashboard/` | The Dashboard: its event feed reads the database and turns new rows into agent, order, money and market events, reads the market streams' counters for their flow, and it assembles the overview |
| | `telegram/` | Telegram bot, alerts and the `telegram.*` tables; `telegram/agent` is the skill-first assistant |
| | `api/` | All HTTP endpoints, JWT auth, rate limiting, one file per module: `user_*.go`, `account_*.go`, `agent_*.go`, `trader_*.go`, `futures_*.go`, `dashboard_*.go` and so on |

Features do not import each other. Where one needs something from another, `cmd/vergex/main.go` passes it in.

Schemas follow the same layers. `core` is under everything, `market` under `account` and `forecast`, `agent` under `trader`, `futures` and `telegram`, and `account` under `trader` and `futures`: see [docs/database.md](docs/database.md).

## Core flows

### Traders

Details are in [docs/traders.md](docs/traders.md). A trader carries its kind and config (`trader.traders`), edited on the Settings section of its page. The entry point is `runCycle` in `internal/trader/auto_trader_loop.go`:

1. Check the running state and hot-reload the settings.
2. Build the trading context: account, positions, candidates. `agent.Engine.GetCandidateCoins` produces candidates from `static`, `volume_top`, `gainers`, `losers`, `oi_top`, `oi_low` or the combined `mixed`. Rankings come from `venue/binance`; OKX traders filter them with `venue/okx`. Check the loss limits: a halt stops opens and is stored on the trader, so it outlives a restart.
3. `trader/agent` fetches candles and indicators, builds the prompts, calls the model and parses decisions.
4. Execute opens and closes by priority, and record orders, positions, decisions and agent usage. Repeated model failures enter safe mode, which opens no new positions; a drawdown monitor goroutine handles emergency closes.

### Futures

Details are in [docs/futures.md](docs/futures.md).

| Endpoint (`/api/futures/`) | Role |
| :-------------------------- | :--- |
| `chat` | The agent. Its tools only read data, draft plans and draw; they cannot place orders |
| `compile` | Compile a plan: risk and reward, fees, sizing |
| `approve` | Lock `plan_hash` and `approval_hash` |
| `execute` | `futures/run` runs a finite, immutable order basket |
| `reconcile`, `check-status` | Reconciliation and recovery |
| `amend-exit`, `cancel-order` | Move an open run's stop or target, placing the new order before cancelling the old; cancel an order no unfinished run holds |
| `candles`, `feed` | Chart history and SSE updates |
| `results` | What finished plans realized, read from the exchange |

`web/src/components/futures/futures-chart/futures-chart.ts` draws plan levels and the agent's drawings (`horizontal_line`, `trend_line`, `rectangle`).

### Market data

Details are in [docs/market-data.md](docs/market-data.md). Each venue's public data is in its own directory under `venue/`, and `market/` and `market/feed` read it from there. Binance's rate limits are per IP and shared with live trading, so live data comes from the streams, and REST through `binance.Limiter` only starts history, fills holes and answers trading when the stream cannot vouch for its data. Calculations and backtests read the local market tables and never make requests. Do not call the exchange from them.

Nothing downstream may hold a stream up: `market/ws` and `Hub` drop rather than wait, and a lost candle leaves its series behind for the hydrator. Whatever trading reads must carry its age, and go to the exchange when it is too old rather than be used silently.

### Forecast

Details are in [docs/forecast.md](docs/forecast.md). `forecast.Evaluate` is pure and takes the data up to a candle, so live forecasts and backtests agree. Keep it that way: anything it reads must come through its arguments, and a change to what it computes raises `EngineVersion`, which on the next start retires the strategy in use and puts a copy to use. Saved strategies never change: a different idea is saved as a new one. Nothing in `trader/` or `futures/` reads forecasts: the Futures chart can overlay one through `/api/forecast/overlay/:symbol`, assembled in `internal/api`, and its web code uses `api/forecast` and the shared chart surface, not `components/forecast`.

## Telegram assistant

`internal/telegram/bot.go` receives updates, and passes each through one gate before anything else: only the user bound with a one-time code from Settings, in their private chat, gets through, checked against what is saved (`binding.go`, `isOwner` in `bot.go`). Anyone else gets a fixed notice in English, and `visitors.go` keeps who they were for Settings to show. With no one bound, the code binds whether it comes after `/start`, as the link sends it, or on its own. `turn.go` runs that chat's messages one at a time, each with a placeholder saying what is under way, and `format.go` turns replies into Telegram's HTML. `internal/telegram/agent` holds the assistant.

The assistant is a trading operations tool, not a general chatbot. The design is 80% fixed skills and 20% dynamic planning: Go code fixes the flow of known, frequent tasks, and the model only understands intent, picks a skill, fills in missing details and writes the reply. User-facing behaviour is in [docs/telegram.md](docs/telegram.md).

`internal/telegram/alerts.go` pushes events to the bound chat and `notify.go` sends them. The watcher only reads, so it adds no exchange requests and never touches the trading loop.

### Commands

`/status`, `/positions` and `/traders` answer from the local API without the model (`agent/commands.go`). `/stop`, `/cancel`, `/lang`, `/help` and `/start` are the rest of the menu, published for the bound chat alone, with `/start` for every private chat (`bot.go`). It goes out before the bot reports running, and a refusal is retried with the bot reporting degraded until then. `/start` answers everyone with where they stand: not bound, access denied or welcome. A command is only for what fixed words can answer: everything else goes to a skill.

### Skills

The model sees only these skills (`internal/telegram/agent/skills.go`):

| Skill | Actions | Notes |
| :---- | :------ | :---- |
| `trader_management` | create, update, delete, start, stop, query | Create takes a `kind` and `preset`, and auto-selects the model or account when only one is enabled. Update sends only `config_patch`; the server merges it |
| `model_management` | query | Read-only; keys and endpoints change only in the web UI |
| `account_management` | query, positions | Read-only, with account connection state; positions reads what an account holds, whoever opened it |
| `account_query` | overview, balance, positions, position_history, trades, orders, open_orders, decisions, decision_detail, statistics, agent_costs | Read-only, per trader. Decisions read the summaries: a full record's prompts would fill a whole result |
| `futures_query` | runs, drafts, results | Read-only, trimmed to what a chat needs; plans are approved and run in the web UI |
| `forecast_query` | signals, detail | Read-only; Forecast is experimental and connected to no trading |
| `close_position` | | Checks first that the position exists |
| `diagnose` | trader_start, model, account, order_execution | Collects facts along fixed paths and gives hints (`diagnose.go`) |

Skills call the local REST API with the bot's JWT over fixed paths, so all server-side validation and risk limits apply. There is no generic `api_request` tool.

### Target resolution (`resolve.go`)

Targets are matched by id, then exact name (case-insensitive), then a unique name substring. Several matches return `ambiguous` with the candidates; no match returns `not_found`. It never guesses. All missing fields are listed at once in `missing_slots`, so the model asks for them in one go.

### Confirming high-risk actions (`confirm.go`)

These actions are held rather than run: starting, stopping or deleting a trader; updating a running trader; `close_position`; and a `config_patch` touching the `RiskFields` of the trader's kind. The tool result returns `needs_confirmation`, and everything one turn holds waits as one batch.

The bot, not the model, shows the batch: `Reply.Confirm` holds Go's summary of each action, with its account and whether that is real money, and `bot.go` sends it under confirm and cancel buttons. Go code decides whether to run it. The user presses a button, whose data names the batch, or replies with an explicit confirmation (`确认`, `confirm`, `yes` and so on); anything else cancels. Confirming runs every action and reports each. A held batch expires after 5 minutes.

### Adding skills

- One skill maps to one complete user task. Don't add a skill for a single endpoint.
- Each action defines its required fields, inference rules, confirmation conditions and failure result shape.
- Route write operations through the `hold()` confirmation gate where it applies. Credential changes (model keys, account keys, Telegram binding, passwords) stay out of the assistant.
- Cover new actions in `agent_test.go`: target resolution, the confirmation gate and the request body.

## Frontend conventions

How the interface looks is in [DESIGN.md](DESIGN.md). This section is the code: where things go and which component does what.

### Structure and copy

- Components go in `web/src/components/<area>/`, pages in `web/src/pages/`. `web/README.md` lists the other directories.
- Keep a file beside its only user, and move it to `hooks/`, `utils/` or `constants/` only when several areas share it. Shared modules must not import from `components/`, `pages/` or `routes/`.
- File and directory names use kebab-case.
- Imports use `./` within a directory and `@/` otherwise, never `../`.
- Use named exports. Use a default export only where a framework requires one.
- UI copy goes through `web/src/i18n`, imported from `@/i18n`, its one entry point. `locales/en` is the template, split by area; `locales/zh-cn` mirrors it file for file, and its types and `tests/i18n/locales.test.ts` fail on a missing key, an extra key or a changed placeholder. To add copy, add it to both, and end it without `.` or `。`, which the same test checks. To add a language, add a folder and list it in `i18n/index.ts`. The language is chosen in the account menu, and otherwise follows the browser. Dates are written in it (`activeLanguage()`); numbers stay in `en-US`.
- New code must pass ESLint and Prettier.
- Tests live in `web/src/tests/`, mirroring the source path after `src/`: the test for `components/ui/button.tsx` is `tests/components/ui/button.test.tsx`. A test that covers several components stays whole, under the directory of the area it covers. Not every file needs a test.

### Reading data

Reads go through TanStack Query's `useQuery`, on the one `queryClient` in `utils/query-client.ts`, which `main.tsx` provides and which is cleared on signing out. A key is an array starting with what is read, such as `['forecast-coins', interval]`, and readers of the same thing share it. A read on a timer spreads `pollEvery(ms)` from `utils/poll.ts`. After a write, `refetch` or `invalidateQueries` what it changed, or `setQueryData` with what the server returned. What several places read is a `queryOptions` of its own, as in `components/forecast/forecast-queries.ts`, which is also what lets `ForecastFrame` and the coin list read ahead. `restoreQueryCache` keeps a short list of Forecast's reads in `localStorage` for the next visit, so the first paint after a reload has data; nothing about accounts, orders or traders goes on that list. Tests render inside `QueryWrapper` from `tests/query-wrapper.tsx`.

### Pages and scrolling

- **Pages** use `components/common/page.tsx`. A list page shows no title or description, which the top bar makes redundant: section tabs and actions share one row, and the title is only read by assistive technology. A detail page passes `detail` for one row with the back link, name, state, context and actions. `leading` puts a search box at the start of the actions row. Content fills the width, with side gutters from `--page-gutter` to match the top bar.
- **Page scrollbars** are drawn by OverlayScrollbars (`utils/page-scroll.ts`) on platforms where native scrollbars take up width: they float and appear while scrolling. Platforms with floating native scrollbars are left alone. Because `html` is what scrolls, lock the page with `lockPageScroll()`, never by changing `overflow` on `body`.
- **Other scroll areas** use `components/ui/scroll-area.tsx` (`ScrollArea`), with the same floating scrollbar and no width taken from content. Put padding in `ScrollArea`'s own `className`; the library moves it to the scroll viewport.

### Panels

A functional area of a page is a `Panel` from `components/common/panel.tsx`. A region being read is `PanelLoading`, its empty and error states are a centred `PanelState`, and a figure being read is a `Skeleton` from `components/ui/skeleton.tsx`. A figure with nothing behind it is 0, and "—" stays for what 0 would misstate: see [DESIGN.md](DESIGN.md). A status line never sits in the page's flow: it floats, or goes on a row that is there anyway, such as `Panel`'s `status`. A choice between a few options shown side by side is `Segmented` from `components/common/segmented.tsx`, tabs inside a page are `SectionTabs` from `components/common/section-tabs.tsx`, a floating panel is `Popover` from `components/ui/popover.tsx`, a list beside its detail is `SplitView` from `components/common/split-view.tsx`, and a record's figures and tables use `StatStrip` and the table classes from `components/common/figures.tsx`. The Dashboard stays read-only: actions belong on the page a row opens.

### Dialogs

Use `components/common/dialog.tsx`, and never hand-write a `fixed inset-0` overlay. To ask before acting, use `confirmToast` from `components/common/notify.tsx`, whose `title` and `okText` name the action and whose `tone: 'danger'` marks a destructive one. `Dialog` provides the overlay, title, close button, Esc, focus trap, page scroll lock, scrolling body and bottom button bar, and mounts on `body`. The file also exports `DialogSteps`, `DialogField`, `DialogNote` and the input style constants.

### Controls

- **Buttons** use `Button` from `components/ui/button.tsx`; for links and other elements use `asChild` or `buttonVariants`. Choose `variant` and `size` by meaning. A one-off style goes in `className` as Tailwind classes, which override defaults through `cn`. Don't style buttons in page CSS or define button style constants.
- **Global element defaults** for `button`, `input`, `select` and `textarea` belong in `styles/base.css`, in `@layer base`; rules outside a layer beat the Tailwind classes on components.
- **Selects** use `AppSelect` from `components/ui/select.tsx` (Radix Select, popover styled like the account menu). A Radix popover in a dialog closes only itself on Esc: it sets `defaultPrevented`, and `Dialog` then stays open. A picker that needs a popover of its own, such as a searchable list, opens it from a `Button` with `variant="select"`, the same trigger. A value that can also be typed, such as a model's name, is the field in `components/settings/model-field.tsx` instead.
- **Switches** use `Switch` from `components/ui/switch.tsx`, a button with `role="switch"`.
- **Search boxes** above long lists use `SearchField` from `components/common/search-field.tsx`, filtering as you type.
- **Number and symbol inputs** in setting rows use `NumberField`, `PeriodsField` and `SymbolsField` from `components/common/form-fields.tsx`, and `FieldGrid` lays rows out in two columns on wide pages.
- **Pointer cursor** is applied to clickable controls globally (`styles/base.css`, `@layer base`), so don't add `cursor-pointer` to each one. A disabled control keeps the default cursor, never `not-allowed`.

### Settings

Models and accounts are configured only in the Settings dialog; they have no pages of their own. Elsewhere, open a section with `SettingsLink` or `useOpenSettings()` (`?settings=models`, `?settings=accounts`). Read this data through `hooks/use-connections.ts`, which shares one cache so saved changes appear at once.

Settings sections, and other forms laid out the same way, use `components/common/setting-row.tsx`:

- **`SettingRow`** for each field: a fixed name column and a control at the standard height filling the rest, with dividers between rows and no helper text under fields.
- **`SettingActions`** ends a group that saves together. Buttons sit on the right. Validation and result messages go in a single line on the left of the bar (`messageTone` sets error, success or in progress, as with the model test result), never under an input, so the layout doesn't jump.
- **`SettingList`** for every window in Settings, lists and forms alike: what it holds scrolls between `head`, such as `SettingBack`, pinned at the top, and the bar with its buttons and any message, pinned at the bottom in the same place on each. A form goes around it so its buttons submit, or its button names the form with `form`. A window with nothing to do passes no `actions` and has no bar.
  - Each entry is a `SettingItem`: icon, name, a `SettingBadge` right after the name, and buttons on the right.
  - Opening one shows its form in the same section, with `SettingBack` at the top.
  - Loading, empty and error states use a centred `SettingState`.

### Motion

Durations and curves are the motion tokens in `styles/tokens.css`, with their values in DESIGN.md. Tailwind's `transition-*` classes default to the fade values. Overlays and pages use `animate-menu-in`, `animate-overlay-in`, `animate-dialog-in` and `animate-page-in`; a menu also sets an `origin-*` class so it grows from what opened it. Hover colour changes apply at once (`hover:duration-0`).

### Colour and themes

- Colour comes only from the tokens in [DESIGN.md](DESIGN.md): in components as the Tailwind classes `vergex-<role>` (`bg-vergex-surface`, `text-vergex-muted`, `border-vergex-line`), in CSS as `var(--<role>)`. No inline hex colours, no Tailwind palette colours such as `bg-white` or `text-gray-500`, and no second set of names.
- Icons come from `lucide-react`. No emoji.
- Light, dark and follow-system themes are supported (`contexts/theme-context.tsx`, `<html data-theme>`). Dark values override the same variables in `:root[data-theme='dark']` in `styles/tokens.css`, so hard-coded colours won't switch. Anything coloured from JS, such as charts, must read the variables and redraw on `THEME_EVENT`.
