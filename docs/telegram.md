# Telegram assistant

A trading operations assistant you reach through your own Telegram bot. It is built around fixed skills rather than free-form tool use: Go code runs the known tasks, and the model only understands the request, picks a skill, fills in missing details and writes the reply.

## Setup

In **Settings → Telegram**:

1. Create a bot with [@BotFather](https://t.me/BotFather) and paste its token.
2. Scan the QR code Settings shows with your phone, or open the binding link on a device with Telegram, and tap Start, which binds the chat at once. Or open the bot yourself and tap Start: it says it is not bound and asks for the code, and sending it the code Settings shows does the same.
3. Choose the model, or leave it on auto-select.

Telegram checks the token as you save it, so a wrong one is refused there and then. A different bot's token starts unbound, since a binding belongs to one bot. Saving the token, model or binding restarts the bot. The step Settings shows comes only from what is saved, so setup runs one way: **Reconfigure**, before anyone is bound, clears the saved token after confirming, stops the bot and goes back to the first step, and the only way forward from there is saving a token. Once bound, the model and alerts apply as you choose them, with nothing left to save.

Settings shows how the bot is really doing, whether it is running or why not, moves on by itself once the chat is bound, and replaces a code that runs out while it is shown, saying how long it has left. A token never appears in errors or logs.

### What Start answers

| Who pressed it | The bot says |
| :------------- | :----------- |
| Anyone, while no one is bound | Not bound, and to send the code from Settings. A wrong or expired code says so |
| Someone else, once someone is bound | Access denied, every time they press it |
| The bound user | Welcome, with what is left to set up or what to do next |

## Bot profile

What people see before they reach the bot is set in [@BotFather](https://t.me/BotFather), not here. This is what suits it:

| Field | Value |
| :---- | :---- |
| Name | `Vergex` |
| About, on the bot's profile page | `Hi 👋~ Here is a self-hosted Vergex trading agent. Private: it only answers its bound owner` |
| Description, under "What can this bot do?" | `Vergex Agent \| Your personal trading assistant. Check accounts and positions, manage traders, read forecast signals and receive alerts. It runs on your own server and answers only the user bound in Settings` |
| Profile photo | [telegram-bot-avatar.png](assets/telegram-bot-avatar.png), 1024 × 1024 |
| Description picture | [telegram-bot-banner.png](assets/telegram-bot-banner.png), 640 × 360 |
| Commands | Leave empty: the bot publishes them itself |
| Allow groups | Off: a group gets no answer anyway |

## Who can use it

The bot works for one Telegram user, in their private chat with it, and that user is whoever bound it. A bot's username is no secret, so binding takes proof that you are the signed-in user: the code in the link or sent on its own, which works once and for 10 minutes, and which a new link replaces. It lives in memory, so a restart means asking for a new one. While someone is bound no new link can be made, so changing who is bound starts with unbinding them. Settings shows who it is, by name and Telegram ID.

Every message and button press passes that check before anything else, commands included. Both the chat and the sender have to be the bound user, as saved at that moment, and the bot asks Telegram for nothing but messages and button presses:

- A group chat gets no answer and cannot bind.
- Anyone else in a private chat gets a short notice in English: access denied, who has access, their own ID and the time. A message gets this at most once a day, and pressing Start every time. You get a system alert naming them.
- With no one bound, only the binding code does anything, and everything else is answered with where things stand. Unbinding in Settings, after confirming it, puts the bot back in that state, from that moment.

A chat bound to a group by an earlier version counts as not bound, and has to be bound again.

**Visitors** in **Settings → Telegram** counts and lists everyone who tried and was refused, groups included: their name, @username and ID, how many times, when last, and the start of their last message, without a binding code. The newest 500 are kept. Binding takes that person off the list.

## Commands

Typing `/` in the bound chat lists them. Every other private chat lists only `/start`, and groups list nothing.

The bot publishes its commands itself when it starts, before it reports running or reads a message, and again when someone binds it or the language changes. If Telegram refuses, Settings says the commands are not ready and why, the bot tries again every 15 seconds, and it still answers commands typed in full.

| Command | Does |
| :------ | :--- |
| `/status` | Each account's balance, how many positions are open and how many traders run |
| `/positions` | What every account holds, whoever opened it |
| `/traders` | Your traders and whether each runs |
| `/stop` | Stops what is running, and what waits behind it |
| `/cancel` | Cancels what is waiting for your confirmation |
| `/lang` | Switches the bot and its alerts between English and Chinese, by button or as `/lang zh` |
| `/help` | What you can ask |
| `/start` | Shows what is set up and what is missing, and begins a new conversation |

The first three answer at once from the local API, without the model, so they cost nothing. They do ask each account for its positions.

## Language

What the bot writes itself, such as commands, requests to confirm and notices, is in the language set with `/lang`. Until one is chosen, it starts from the language Settings was in when you bound the chat, and otherwise moves to Chinese the first time you write in it. The model is told the setting, and follows you if you write in another language. Error details from an exchange or provider stay as they came.

A chat is stored in `agent.conversations` and `agent.messages`: what you and the assistant said, the account state it was given, its thinking where the model returns one, and each skill call with its result. After a restart the assistant picks up from the latest 40 things said. `/start` begins a new conversation; the old one stays stored, and nothing cleans it up automatically (see [Data and backups](getting-started.md#data-and-backups)).

## Skills

Defined in `internal/telegram/agent/skills.go`:

| Skill | Actions |
| :---- | :------ |
| `trader_management` | create, with a kind and preset; update, with a `config_patch` the server merges; delete, start, stop, query |
| `model_management` | query |
| `account_management` | query, with account connection state; positions, what an account holds whoever opened it |
| `account_query` | overview, balance, positions, position history, trades, orders, open orders, decisions (the latest five cycles, without their prompts), one cycle's detail with its reasoning, statistics, agent costs |
| `futures_query` | read only: runs, drafts, and what finished plans realized on an account |
| `forecast_query` | read only: signals for the watched coins, or one coin in detail |
| `close_position` | closes a position after checking that it exists |
| `diagnose` | `trader_start`, `model`, `account`, `order_execution` |

Skills call the local REST API with fixed paths, so server-side validation and risk limits always apply. Keys and endpoints for models and accounts can only be changed in the web UI.

## Confirmation

These actions wait for your confirmation: starting, stopping or deleting a trader, updating a running trader, closing a position, and a `config_patch` that touches the risk fields of the trader's kind.

The bot itself lists what is waiting, with the account each action is on and whether that is real money, under confirm and cancel buttons. The model writes none of it, so what you confirm is what you read. A request that holds several actions, such as stopping every trader, waits as one: confirming runs them all and says how each went, and one failing does not stop the rest.

The check runs in Go, not in the model. Press a button, or reply with an explicit confirmation (`confirm`, `yes`, `确认`); anything else cancels, and it expires after 5 minutes. A button only answers the request it came with, so an old one cannot approve anything newer.

## While it works

Messages are answered one at a time, in the order they came. While the assistant works, a placeholder says what it is doing now, such as reading positions, in the bot's own words, never the model's or a tool's raw data. Once the model is writing its answer, the placeholder shows that as it comes, unformatted, and the finished reply takes its place. It updates at most once a second, and follows Telegram's rate limits.

The placeholder has a stop button, and `/stop` does the same at any time: it stops the turn and drops whatever waits behind it. What was already done stays done, and what the turn was holding for confirmation goes with it. A turn has 3 minutes at most. One that runs out of time, or out of steps, says it did not finish and never that it did.

## Replies

Replies are written for a phone. What the model writes in Markdown becomes Telegram's own formatting: bold, lists, code and links. A table becomes one entry per row, since columns do not fit, and a reply too long for one message arrives in several. If Telegram refuses the formatting, the same words go out as plain text.

## Finding the target

Targets are matched by id, then exact name (case-insensitive), then a unique part of the name. If several match, the assistant asks you to choose; if none match, it says so. It never guesses.

## Alerts

Once the bot is bound, the assistant also tells you about events without being asked. **Settings → Telegram** has a pill for each kind: pressed, it is sent, and otherwise it is not. **Send test** checks that messages arrive.

| Kind | Sent when |
| :--- | :-------- |
| Rejected orders | The exchange rejects a trader's order, or an order action fails. Actions a risk guard stopped are not reported |
| Safe mode | A trader enters safe mode or trading is blocked. Sent once each time it begins |
| Closed positions | A trader's or a Futures position closes, with its realized P/L |
| Futures plans | A plan's entry fills, or a plan needs reconciliation, including after a restart |
| Forecast signals | A watchlist coin's score crosses the threshold, on an interval picked in Forecast → Settings. See [Forecast](forecast.md#alerts) |
| Daily report | Just after 00:00 UTC, the day before on your real accounts: their equity now, what was realized and what is open. Testnet accounts count for nothing, and with no real account nothing is sent. It also gives what all model calls cost that day, since a call belongs to no one account |
| System problems | A background service fails, a trader halts on a loss limit or cannot start, the ledger cannot book a trade, or an account's reads keep failing for 5 minutes. Sent once each time it begins |

`internal/telegram/alerts.go` checks every 30 seconds. It only reads the database and in-memory state, so it adds no exchange requests and does not touch the trading loop. The one exception is the daily report, which asks each real account its equity once a day. What was already sent is stored in `telegram.alert_marks`, so a restart repeats nothing. Connecting the bot does not replay history, and after downtime events are reported from at most 24 hours back, each saying when it happened. If a message cannot be delivered, what did not get through goes out on the next check. A trader on a testnet account is marked as such.

## Diagnostics

`diagnose` follows a fixed path for each topic (`internal/telegram/agent/diagnose.go`). It reports the facts it collected with a hint for each known failure:

- **`trader_start`:** preflight checks for the trader's model, settings and account.
- **`model`:** whether an enabled model exists and is complete.
- **`account`:** account state, such as missing or rejected credentials, missing permissions, or an unreachable exchange.
- **`order_execution`:** recent orders and their failures.

Ask in plain language, for example "why won't my trader start?". It helps to separate three cases: the trader did not start, it is running but the model chose not to trade, and it tried to place an order that failed.
