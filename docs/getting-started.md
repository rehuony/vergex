# Getting started

## Run with Docker

Requirements: Docker with Compose, `make`, `openssl`.

```bash
make up
```

The first run creates `.env` from `.env.example`, generates `JWT_SECRET`, `DATA_ENCRYPTION_KEY`, `RSA_PRIVATE_KEY` and `DB_PASSWORD`, then builds and starts the containers, including PostgreSQL. Open http://127.0.0.1:8000.

`make help` lists every command. The ones you will use most:

| Command | What it does |
| :------ | :----------- |
| `make status` | Container state and the health endpoint |
| `make logs SERVICE=vergex-backend` | Follow the backend log |
| `make down` | Stop and remove the containers; the database volume is kept |
| `make regen-keys` | Regenerate all keys. Stored account and model keys become unreadable |

## Run from source

Requirements: the Go version in `go.mod`, Docker for PostgreSQL, Node.js 24, and pnpm (`corepack enable` once; the version is pinned in `web/package.json`).

```bash
make init          # once, for the keys in .env
make deps          # Go modules and frontend packages
make dev           # PostgreSQL from .env, backend on :3000, frontend on :8000 with /api proxied to :3000
```

`make run-postgresql` starts PostgreSQL with the settings in `.env`: the same container `make up` uses, also published on `127.0.0.1` at `DB_PORT` for the backend and tests running from source. `make test` runs the Go tests and the frontend tests. Each Go test gets a database of its own, cloned from the template `<DB_NAME>_test` and dropped when it ends, so a database tool may show those clones briefly while tests run. Set `VERGEX_TEST_DATABASE_URL` to use another server: the tests drop and rebuild the database it names, so that name must end in `_test`, and they refuse any other. `make build` produces `./vergex` and `web/dist`.

## First session

The interface is in English or Simplified Chinese. It follows your browser until you choose one from the account menu.

1. **Create the user.** The first registered user is the only user of the instance.
2. **Add a model** in Settings → Agent models: see [models.md](models.md).
3. **Add an account** in Settings → Trading accounts: see [accounts.md](accounts.md). A [testnet](accounts.md#testnet) account lets you try everything with test money.
4. **Plan a trade** in [Futures](futures.md), which asks for your approval on every order, or create a [trader](traders.md), which places orders on its own once started.

Stopping the service does not close positions. Check the exchange after a restart.

## Configuration

Settings come from `.env`; `.env.example` documents each one. Models, accounts and traders are stored in the database and edited in the web UI.

| Setting | Default | Notes |
| :------ | :------ | :---- |
| `VERGEX_BIND_ADDRESS`, `VERGEX_FRONTEND_PORT` | `127.0.0.1`, `8000` | Where the web UI and its `/api/` proxy listen |
| `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `vergex`, generated, `vergex`, `disable` | PostgreSQL. `make run-postgresql` and `make up` start it with these values |
| `DB_HOST`, `DB_PORT` | `127.0.0.1`, `5432` | Where the backend and tests find the database when running from source, and the port `make run-postgresql` publishes. `make up` connects its containers over their own network instead, so start the stack with it rather than plain `docker compose up` |
| `TRANSPORT_ENCRYPTION` | `false` | Encrypt API keys in the browser before sending; needs HTTPS or localhost |
| `CORS_ALLOWED_ORIGINS` | localhost origins | Only needed when the UI is served from another origin |

The schema is migrated automatically at startup, by the SQL files in `internal/database/migrations`. Migrations only go forward: see [database.md](database.md).

## Data and backups

Everything the backend keeps lives in PostgreSQL, in the `vergex-postgres` Docker volume; it writes no files. Logs go to standard output, so `make logs` shows them, and they last as long as the container. Back up the database with:

```bash
make backup
```

It writes `backups/vergex-<time>.sql.gz`, readable only by you. Account and model keys and the Telegram bot token are stored AES-encrypted with `DATA_ENCRYPTION_KEY`, so back up `.env` and the database together and protect both.

To restore, name the file and a database that does not exist yet:

```bash
make restore FILE=backups/vergex-20261102-150000.sql.gz DB=vergex_restored
```

A restore never overwrites anything: it fills the new database, or removes it again if it fails. To run on it, stop the backend, set `DB_NAME=vergex_restored` in `.env`, and start again with the `.env` the backup was made with. Open positions and orders stay on the exchange whatever the backup says, and the account sync brings the ledger up to date from there.

What the models were sent and what they said is kept in `agent.messages` and never cleaned up automatically, not even when a trader is deleted. **Settings → Agent costs** shows how much disk it takes. These figures are estimates:

| From | Growth |
| :--- | :----- |
| A trader | 10–22 KB a cycle. For one trader, 0.35–0.74 GB a year at the default 15-minute interval, 1.7–3.7 GB a year at 3 minutes |
| Futures | About 39 KB of workspace and market data per turn, plus the words and tool results |
| Telegram | Not measured |

To clear out old messages, see first what would go, then confirm:

```bash
docker exec -it vergex-backend ./vergex prune --before 90d
```

```bash
docker exec -it vergex-backend ./vergex prune --before 90d --yes
```

From source, run the same subcommand with `go run ./cmd/vergex`. It only touches `agent.messages`: conversations and cycles stay listed, without the text, and orders, positions, costs and forecasts stay whole. `--before` takes at least a day. PostgreSQL reuses the freed space for new messages, so the files on disk don't shrink.

## User recovery

Recovery is only available on the host, never over HTTP:

```bash
docker exec -it vergex-backend ./vergex reset-password --email you@example.com
docker exec -it vergex-backend ./vergex reset-user
```

`reset-user` permanently deletes the user and their data. From source, run the same subcommands with `go run ./cmd/vergex`.
