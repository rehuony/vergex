# Troubleshooting

## First checks

```bash
make status                          # container state and health
make logs SERVICE=vergex-backend     # backend log
curl http://127.0.0.1:8000/api/health
```

Logs also go to `data/vergex_YYYY-MM-DD.log`. The [Telegram assistant](telegram.md) can run a diagnosis for a trader, model or exchange. For a problem in the UI, check the browser console and network tab.

## Starting up

**The backend exits at startup.** It refuses to start with a missing or default `JWT_SECRET`. Run `make init` to fill in missing keys.

**Stored keys can't be read after a change.** `DATA_ENCRYPTION_KEY` changed. Restore the old value from your backup; otherwise re-enter the exchange and model keys.

**Port already in use.** Find the process with `lsof -i :8000` (`:3000` when running from source), or set `VERGEX_FRONTEND_PORT` in `.env`.

**`database is locked`.** Two processes are using the same SQLite file. Stop the other one; `make down` stops the containers.

**The UI loads forever.** The frontend can't reach the backend. Check `make status` and the backend log.

**Docker Hub is unreachable.** `docker compose up --build` hangs on `load metadata`. Configure a registry mirror under `registry-mirrors` in Docker's `daemon.json`, restart Docker and try again.

## Exchanges

See [exchanges.md](exchanges.md) for API key errors.

**`-1021`, `invalid signature`, timestamp errors.** The server clock is off. Sync it with `sudo timedatectl set-ntp true`, and compare `docker exec vergex-backend date` against the host. If the error persists, check the key's permissions and IP whitelist.

**Rate limits.** Lengthen the scan interval or run fewer traders.

## Autopilot

**The trader won't start.** Starting it lists each launch check with the reason any of them failed. An account held by an unresolved Frontier plan also blocks the start.

**The trader runs but doesn't trade.** This is usually the model's decision, not a fault: the Decisions section of the trader's page shows its latest decisions and reasoning. Other causes:

- max positions reached, or not enough margin
- a just-closed symbol in its 4-hour re-entry cooldown
- safe mode after 3 AI failures in a row

**It only opens one side, or fails with `-4061`.** The Binance account is in One-way Mode: see [exchanges.md](exchanges.md).

**Leverage errors.** Sub-accounts can be limited, for example to 5x. Lower the leverage in the strategy.

**A strategy change has no effect.** Check that it was saved and that the trader uses this strategy. Strategies reload on the next cycle. A strategy with `language` set to `zh` ignores its custom prompt.

## Models

**AI unavailable, or a model call fails.** Check that the model is enabled and has an API key, then use the test button in Settings → AI models.

**A custom URL is rejected.** Localhost and private addresses are blocked, and the URL must not include `/chat/completions` unless it ends with `#`: see [models.md](models.md).

## Frontier

**A run shows Needs attention.** Check positions and orders at the exchange first, then use **Reconcile exchange state**. See [frontier.md](frontier.md).

**The chart doesn't update.** Behind a reverse proxy, make sure buffering is off for SSE: see [deployment.md](deployment.md).

## Starting over

```bash
docker exec -it vergex-backend ./vergex reset-account
```

This deletes the account and its data. To remove everything, run `make down` and delete `data/`. Back up `.env` and `data/` first. Neither step closes positions on the exchange.
