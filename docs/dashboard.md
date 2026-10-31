# Dashboard

The Dashboard is a read-only view of the whole system, built around where things are going right now. Nothing on it changes anything: each row opens the page where it can be acted on.

| Area | Shows | Opens |
| :--- | :---- | :---- |
| Needs attention | Blocked or safe-mode traders, traders that could not load, accounts that cannot be read, Frontier plans needing reconciliation, stopped services. One row per kind of problem, leading to the first. Hidden when all is well | The trader, Frontier, or the matching Settings section |
| Scene | Every block of the system, its members as dots, and the routes between them | Filters the events |
| Events | What happened in the last 24 hours, newest first, up to 200 | The decision, the trader's Orders or Positions section, the Frontier plan, Forecast, or Settings |
| Positions | Every open position across accounts, and what holds it | The trader's Positions section, or Frontier |

Until an AI model and an exchange account exist, the page shows the three steps to get started instead.

## Scene

| Block | One dot is |
| :---- | :--------- |
| Market | A coin market data follows |
| Telegram | The bound chat |
| Traders | A trader. Hollow when stopped |
| Frontier | An active plan. Red when it needs reconciliation |
| Forecast | A coin the strategy in use has forecast |
| Models | A host that AI models call, from each model's URL |
| Accounts | An exchange account. Hollow when it cannot be read. The figure is the total equity of those that can |
| Positions | An open position, long above and short below, coloured by unrealized P/L |

A block that has more members than fit shows `+N` beside its name. On a phone the blocks become a list, without routes.

Each event lights its route once, in the direction it went, and nothing moves without one. The line's colour says what it was:

| Colour | Event |
| :----- | :---- |
| Accent | AI: a call out and back (prompt and reply), or a decision coming back to its trader |
| Primary text | An order, or margin moving into a position |
| Grey | A market read: a trader's cycle, or forecasts for a closed candle |
| Green | Realized profit returning to an account |
| Red | A loss, a fee, an AI cost, or anything that failed |

A burst of events on one route draws once with a count beside it, and at most 12 lines run at a time. Under reduced motion a route lights up in place instead of travelling. An AI call outside the scene, such as a model or strategy test, is only in the event list.

Hovering a block or route shows its members or its figures for the last hour and day in the bar below the scene; clicking one filters the events and dims the rest. The bar otherwise shows the day's totals. If the stream drops, the page keeps what it has, says so in the bar and reconnects.

## Data

`GET /api/activity/stream` serves the events as server-sent events. A feed (`internal/activity`) reads the database every 3 seconds and turns new rows into events, so it adds no exchange requests and never touches the trading loop. It keeps the latest 500 in memory, rebuilds from the last 24 hours of the tables when it starts, and keeps exact totals per route.

| Event | Read from | Limits |
| :---- | :-------- | :----- |
| AI call | `ai_charges`, with who made the call and the host it went to. Never the key | Recorded when a call finishes, so a failed call appears only through its decision |
| AI cost | `ai_charges` | An estimate from models.dev prices. A call that cannot be priced shows none |
| Decision, market read | `decision_records` | Saved when a cycle ends, so times are worked back from its actions |
| Order | `trader_orders` | One event per status it is seen in |
| Frontier order | `planning_live_runs` | Has no time of its own, so it takes the run's. Only once it reaches the exchange |
| Fee | `trader_fills` | A fee not in a dollar stablecoin counts as an event but adds nothing to the route's total |
| Margin, realized P/L | `trader_positions` | Traders only: Frontier's positions are not stored |
| Forecast | `forecast_records` | Grouped per interval |

Frontier's charts read market data directly, without recording it, so there is no route from Market to Frontier. Deposits, withdrawals and transfers are not shown: nothing reads them yet.

`GET /api/overview` builds the blocks and Positions. It reads balances, positions and open orders from the exchanges, whose request allowance is per IP and shared with live trading, so the result is cached for 15 seconds per user, concurrent requests are merged, and the page polls every 20 seconds only while visible. Balances use the existing 30-second account state cache.
