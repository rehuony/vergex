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
| `make down` | Stop and remove the containers; the database volume and `data/` are kept |
| `make regen-keys` | Regenerate all keys. Stored exchange and model keys become unreadable |

## Run from source

Requirements: the Go version in `go.mod`, Docker for PostgreSQL, Node.js 24, and pnpm (`corepack enable` once; the version is pinned in `web/package.json`).

```bash
make init          # once, for the keys in .env
make deps          # Go modules and frontend packages
make dev           # PostgreSQL from .env, backend on :3000, frontend on :8000 with /api proxied to :3000
```

`make run-postgresql` starts PostgreSQL with the settings in `.env`: the same container `make up` uses, also published on `127.0.0.1` at `DB_PORT` for the backend and tests running from source. `make test` runs the Go tests in a `<DB_NAME>_test` database on it, each in a schema of its own, and the frontend tests; set `VERGEX_TEST_DATABASE_URL` to use another server. `make build` produces `./vergex` and `web/dist`.

## First session

1. **Create the account.** The first registered user is the only user of the instance.
2. **Add an AI model** in Settings → AI models: see [models.md](models.md).
3. **Add an exchange account** in Settings → Exchanges: see [exchanges.md](exchanges.md).
4. **Plan a trade** in [Frontier](frontier.md), which asks for your approval on every order, or create a [strategy and a trader](traders.md), which places orders on its own once started.

Stopping the service does not close positions. Check the exchange after a restart.

## Configuration

Settings come from `.env`; `.env.example` documents each one. Models, exchange accounts, traders and strategies are stored in the database and edited in the web UI.

| Setting | Default | Notes |
| :------ | :------ | :---- |
| `VERGEX_BIND_ADDRESS`, `VERGEX_FRONTEND_PORT` | `127.0.0.1`, `8000` | Where the web UI and its `/api/` proxy listen |
| `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `vergex`, generated, `vergex`, `disable` | PostgreSQL. `make run-postgresql` and `make up` start it with these values |
| `DB_HOST`, `DB_PORT` | `127.0.0.1`, `5432` | Where the backend and tests find the database when running from source, and the port `make run-postgresql` publishes. `make up` connects its containers over their own network instead, so start the stack with it rather than plain `docker compose up` |
| `TRANSPORT_ENCRYPTION` | `false` | Encrypt API keys in the browser before sending; needs HTTPS or localhost |
| `CORS_ALLOWED_ORIGINS` | localhost origins | Only needed when the UI is served from another origin |

The schema is migrated automatically at startup with GORM AutoMigrate.

## Data and backups

Relational data lives in PostgreSQL, in the `vergex-postgres` Docker volume. `data/` holds daily logs (`vergex_YYYY-MM-DD.log`), the cached model price list and Frontier's entry calculators. Back up the database with:

```bash
docker exec vergex-postgres pg_dump -U vergex vergex > vergex.sql
```

Exchange and model keys are stored AES-encrypted with `DATA_ENCRYPTION_KEY`, so back up `.env`, `data/` and the database together and protect both.

## Account recovery

Recovery is only available on the host, never over HTTP:

```bash
docker exec -it vergex-backend ./vergex reset-password --email you@example.com
docker exec -it vergex-backend ./vergex reset-account
```

`reset-account` permanently deletes the account and its data. From source, run the same subcommands with `go run ./cmd/vergex`.
