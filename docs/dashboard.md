# Dashboard

The Dashboard is a read-only view of the whole system, built around where things are going right now. Nothing on it changes anything: each row opens the page where it can be acted on.

| Area | Shows | Opens |
| :--- | :---- | :---- |
| Needs attention | Blocked or safe-mode traders, traders that could not load, accounts that cannot be read, Frontier plans needing reconciliation, stopped services. One row per kind of problem, leading to the first. Hidden when all is well | The trader, Frontier, or the matching Settings section |
| Scene | Every block of the system, its members as dots, and the routes between them | Filters the events |
| Events | What happened in the last 24 hours, newest first, up to 200 | The decision, the trader's Orders or Positions section, the Frontier plan, Forecast, or Settings |
| Positions | Every open position across accounts, and what holds it | The trader's Positions section, or Frontier |

Until a model and an exchange account exist, the page shows the three steps to get started instead.

## Scene

| Block | One dot is |
| :---- | :--------- |
| Market | A coin market data follows |
| Telegram | The bound chat |
| Traders | A trader. Hollow when stopped |
| Frontier | An active plan. Red when it needs reconciliation |
| Forecast | A coin the strategy in use has forecast |
| Models | A host that models are called on, from each model's URL |
| Accounts | An exchange account. Hollow when it cannot be read. The figure is the total equity of those that can |
| Positions | An open position, long above and short below, coloured by unrealized P/L |

A block that has more members than fit shows `+N` beside its name. On a phone the blocks become a list, without routes.

Each event lights its route once, in the direction it went, and nothing moves without one. The line's colour says what it was:

| Colour | Event |
| :----- | :---- |
| Accent | Agent: a call out and back (prompt and reply), or a decision coming back to its trader |
| Primary text | An order, or margin moving into a position |
| Grey | A market read: a trader's cycle, or forecasts for a closed candle |
| Green | Realized profit returning to an account |
| Red | A loss, a fee, an agent cost, or anything that failed |

A burst of events on one route draws once with a count beside it, and at most 12 lines run at a time. Under reduced motion a route lights up in place instead of travelling. A model call outside the scene, such as a model or strategy test, is only in the event list.

Hovering a block or route shows its members or its figures for the last hour and day in the bar below the scene; clicking one filters the events and dims the rest. The bar otherwise shows the day's totals. If the stream drops, the page keeps what it has, says so in the bar and reconnects.

## Data

`GET /api/activity/stream` serves the events as server-sent events. A feed (`internal/activity`) reads the database every 3 seconds and turns new rows into events, so it adds no exchange requests and never touches the trading loop. It keeps the latest 500 in memory, rebuilds from the last 24 hours of the tables when it starts, and keeps exact totals per route. Names come from columns that hold no key, so the feed decrypts nothing.

| Event | Read from | Limits |
| :---- | :-------- | :----- |
| Model call | `agent.calls`, with who made the call and the host it went to. Never the key | Recorded when a call finishes. A failed attempt shows as failed, at no cost, and each retry is its own event |
| Agent cost | `agent.calls` | An estimate from models.dev prices. A call that cannot be priced shows none |
| Decision, market read | `trader.cycles`, `trader.decisions` | Saved when a cycle ends, so times are worked back from its actions |
| Order | `trading.orders` | One event per status it is seen in, at the time it got there rather than when the ledger heard, so a sync catching up after an outage is not news. Orders from Frontier and orders nothing claims show on their own routes |
| Frontier order | `frontier.run_orders` | Only an order refused before it reached the exchange, which the ledger never hears of, at the time its row last changed |
| Fee | `trading.fills` | A fee not in a dollar stablecoin counts as an event but adds nothing to the route's total |
| Margin, realized P/L | `trading.positions` | Every source, Frontier's included. Leverage comes from the order that opened the position, so a position known only through its fills shows no margin |
| Forecast | `forecast.signals` | Grouped per interval |

Frontier's charts read market data directly, without recording it, so there is no route from Market to Frontier. Deposits, withdrawals and transfers are not shown: nothing reads them yet.

`GET /api/overview` builds the blocks and Positions. It reads balances, positions and open orders from the exchanges, whose request allowance is per IP and shared with live trading, so the result is cached for 15 seconds per user, concurrent requests are merged, and the page polls every 20 seconds only while visible. Balances use the existing 30-second account state cache. Realized today sums positions closed since 00:00 UTC on the user's accounts, whoever traded them: a trader, Frontier or a manual close.
