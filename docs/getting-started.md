# Getting started

## Run with Docker

Requirements: Docker with Compose, `make`, `openssl`.

```bash
make up
```

The first run creates `.env` from `.env.example`, generates `JWT_SECRET`, `DATA_ENCRYPTION_KEY` and `RSA_PRIVATE_KEY`, then builds and starts the containers. Open http://127.0.0.1:8000.

`make help` lists every command. The ones you will use most:

| Command | What it does |
| :------ | :----------- |
| `make status` | Container state and the health endpoint |
| `make logs SERVICE=vergex-backend` | Follow the backend log |
| `make down` | Stop and remove the containers; `data/` is kept |
| `make regen-keys` | Regenerate all keys. Stored exchange and model keys become unreadable |

## Run from source

Requirements: the Go version in `go.mod` (CGO is needed for SQLite), Node.js 24, and pnpm (`corepack enable` once; the version is pinned in `web/package.json`).

```bash
make init          # once, for the keys in .env
make deps          # Go modules and frontend packages
make dev           # backend on :3000, frontend on :8000 with /api proxied to :3000
```

`make test` runs the Go and frontend tests. `make build` produces `./vergex` and `web/dist`.

## First session

1. **Create the account.** The first registered user is the only user of the instance.
2. **Add an AI model** in Settings → AI models: see [models.md](models.md).
3. **Add an exchange account** in Settings → Exchanges: see [exchanges.md](exchanges.md).
4. **Plan a trade** in [Frontier](frontier.md), which asks for your approval on every order, or create a strategy and a trader in [Autopilot](autopilot.md), which places orders on its own once started.

Stopping the service does not close positions. Check the exchange after a restart.

## Configuration

Settings come from `.env`; `.env.example` documents each one. Models, exchange accounts, traders and strategies are stored in the database and edited in the web UI.

| Setting | Default | Notes |
| :------ | :------ | :---- |
| `VERGEX_BIND_ADDRESS`, `VERGEX_FRONTEND_PORT` | `127.0.0.1`, `8000` | Where the web UI and its `/api/` proxy listen |
| `DB_TYPE` | `sqlite` | `postgres` also needs `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` |
| `DB_PATH` | `data/data.db` | SQLite file |
| `TRANSPORT_ENCRYPTION` | `false` | Encrypt API keys in the browser before sending; needs HTTPS or localhost |
| `CORS_ALLOWED_ORIGINS` | localhost origins | Only needed when the UI is served from another origin |

The schema is migrated automatically at startup with GORM AutoMigrate, on both SQLite and PostgreSQL.

## Data and backups

Everything lives in `data/`: the SQLite database, daily logs (`vergex_YYYY-MM-DD.log`) and the cached model price list. Exchange and model keys are stored AES-encrypted with `DATA_ENCRYPTION_KEY`, so back up `.env` and `data/` together and protect both.

## Account recovery

Recovery is only available on the host, never over HTTP:

```bash
docker exec -it vergex-backend ./vergex reset-password --email you@example.com
docker exec -it vergex-backend ./vergex reset-account
```

`reset-account` permanently deletes the account and its data. From source, run the same subcommands with `go run ./cmd/vergex`.
