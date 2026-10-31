# Telegram assistant

A trading operations assistant you reach through your own Telegram bot. It is built around fixed skills rather than free-form tool use: Go code runs the known tasks, and the model only understands the request, picks a skill, fills in missing details and writes the reply.

## Setup

In **Settings → Telegram**:

1. Create a bot with [@BotFather](https://t.me/BotFather) and paste its token.
2. Send `/start` to your bot. It binds to your account.
3. Choose the AI model, or leave it on auto-select.

Saving the token, model or binding restarts the bot.

## Skills

Defined in `internal/telegram/agent/skills.go`:

| Skill | Actions |
| :---- | :------ |
| `trader_management` | create, update, delete, start, stop, query |
| `strategy_management` | create, update, delete, activate, duplicate, query |
| `model_management` | query |
| `exchange_management` | query, with account connection state |
| `account_query` | overview, balance, positions, position history, trades, orders, open orders, decisions, statistics, AI costs |
| `close_position` | closes a position after checking that it exists |
| `diagnose` | `trader_start`, `model`, `exchange`, `order_execution` |

Skills call the local REST API with fixed paths, so server-side validation and risk limits always apply. Keys and endpoints for models and exchanges can only be changed in the web UI.

## Confirmation

These actions wait for your confirmation: starting, stopping or deleting a trader, updating a running trader, closing a position, deleting a strategy, and changing a strategy's `risk_control`, `grid_config` or `strategy_type`.

The check runs in Go, not in the model. Your next message must be an explicit confirmation (`confirm`, `yes`, `确认`); anything else cancels the action, and it expires after 5 minutes.

## Finding the target

Targets are matched by id, then exact name (case-insensitive), then a unique part of the name. If several match, the assistant asks you to choose; if none match, it says so. It never guesses.

## Alerts

Once the bot is bound, the assistant also tells you about events without being asked. **Settings → Telegram** turns each kind on or off, and **Send test** checks that messages arrive.

| Kind | Sent when |
| :--- | :-------- |
| Rejected orders | The exchange rejects a trader's order, or an order action fails. Actions a risk guard stopped are not reported |
| Safe mode | A trader enters safe mode or trading is blocked. Sent once each time it begins |
| Closed positions | A trader's position closes, with its realized P/L |
| Frontier plans | A plan needs reconciliation, including after a restart |
| Forecast signals | A watchlist coin's score crosses the threshold in Settings → Forecast. See [Forecast](forecast.md#alerts) |

`internal/telegram/alerts.go` checks every 30 seconds. It only reads the database and in-memory state, so it adds no exchange requests and does not touch the trading loop. What was already sent is stored in `telegram_alert_marks`, so a restart repeats nothing. Connecting the bot does not replay history, and after downtime events are reported from at most 24 hours back. If a message cannot be delivered, the same events go out on the next check.

## Diagnostics

`diagnose` follows a fixed path for each topic (`internal/telegram/agent/diagnose.go`). It reports the facts it collected with a hint for each known failure:

- **`trader_start`:** preflight checks for the trader's model, exchange account and strategy.
- **`model`:** whether an enabled model exists and is complete.
- **`exchange`:** account state, such as missing or rejected credentials, missing permissions, or an unreachable exchange.
- **`order_execution`:** recent orders and their failures.

Ask in plain language, for example "why won't my trader start?". It helps to separate three cases: the trader did not start, it is running but the model chose not to trade, and it tried to place an order that failed.
