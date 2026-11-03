# Troubleshooting

## First checks

```bash
make status                          # container state and health
make logs SERVICE=vergex-backend     # backend log
curl http://127.0.0.1:8000/api/health
```

A log lasts as long as its container: `make up` after a change starts a new one. The [Telegram assistant](telegram.md) can run a diagnosis for a trader, model or account. For a problem in the UI, check the browser console and network tab.

## Starting up

**The backend exits at startup.** It refuses to start with a missing or default `JWT_SECRET`. Run `make init` to fill in missing keys.

**Stored keys can't be read after a change.** `DATA_ENCRYPTION_KEY` changed. Restore the old value from your backup; otherwise re-enter the account and model keys.

**`migration 0002_core.sql changed after it was applied` at startup.** The database was built from an older baseline, which this version replaced without carrying data over, as it may until the schema settles. `cannot drop schema public` is the same for a database from before the schemas. Back up anything you need, then rebuild the database as [database.md](database.md#starting-from-the-baseline) describes.

**Port already in use.** Find the process with `lsof -i :8000` (`:3000` when running from source), or set `VERGEX_FRONTEND_PORT` in `.env`. If `make run-postgresql` finds its port taken, set `DB_PORT` in `.env`.

**`failed to open PostgreSQL database`.** The backend can't reach the database. Check `make status`; from source, `make run-postgresql` starts it with the settings in `.env`, and `make run-backend` does so first.

**The UI loads forever.** The frontend can't reach the backend. Check `make status` and the backend log.

**Docker Hub is unreachable.** `docker compose up --build` hangs on `load metadata`. Configure a registry mirror under `registry-mirrors` in Docker's `daemon.json`, restart Docker and try again.

## Accounts

See [accounts.md](accounts.md) for API key errors.

**`-1021`, `invalid signature`, timestamp errors.** The server clock is off. Sync it with `sudo timedatectl set-ntp true`, and compare `docker exec vergex-backend date` against the host. If the error persists, check the key's permissions and IP whitelist.

**Rate limits.** Lengthen the scan interval or run fewer traders.

## Auto trading

**The trader won't start.** Starting it lists each launch check with the reason any of them failed. An account held by an unresolved Futures plan also blocks the start.

**The trader runs but doesn't trade.** This is usually the model's decision, not a fault: a trader's Decisions tab shows its latest decisions and reasoning. Other causes:

- max positions reached, or not enough margin
- a just-closed symbol in its 4-hour re-entry cooldown
- safe mode after 3 model failures in a row

**It only opens one side, or fails with `-4061`.** The Binance account is in One-way Mode: see [accounts.md](accounts.md).

**Leverage errors.** Sub-accounts can be limited, for example to 5x. Lower the leverage in the trader's settings.

**A settings change has no effect.** Check that it was saved. A running agent trader picks it up on its next cycle, and a running grid trader restarts with it. A prompt section written in Chinese is ignored, since system prompts are English only.

## Models

**A model call fails.** Check that the model is enabled and has an API key, then use the test button in Settings → Agent models. What you are told says why, with the provider's own line; the full error of every failed call is in `agent.calls`. See [When a call fails](models.md#when-a-call-fails).

**A custom URL is rejected.** Localhost and private addresses are blocked, and the URL must not include `/chat/completions` unless it ends with `#`: see [models.md](models.md).

## Futures

**A run shows Needs attention.** Check positions and orders at the exchange first, then use **Reconcile exchange state**. See [futures.md](futures.md).

**The chart doesn't update.** Behind a reverse proxy, make sure buffering is off for SSE: see [deployment.md](deployment.md).

## Starting over

```bash
docker exec -it vergex-backend ./vergex reset-user
```

This deletes the user and their data. To remove everything, run `docker compose down -v`. Back up `.env` and the database first. Neither step closes positions on the exchange.
