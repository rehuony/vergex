# Dashboard

The Dashboard is a read-only view of the whole system, built around where things are going right now. Nothing on it changes anything: each row opens the page where it can be acted on.

| Area | Shows | Opens |
| :--- | :---- | :---- |
| Figures | Total equity, what was realized today, traders running and open positions, above the scene | |
| Needs attention | The last of the figures, always there. It says None, or how many problems there are and opens a list of them: blocked or safe-mode traders, traders that could not load, accounts that cannot be read, Futures plans needing reconciliation, stopped services. One row per kind of problem, leading to the first | The trader, Futures, or the matching Settings section |
| Scene | Every block of the system, its members as dots on top, and the routes between them | Filters the events |
| Events | What happened in the last 24 hours, newest first, up to 200 | The decision, the trader's Orders or Positions section, the Futures plan, Forecast, or Settings |
| Positions | The last choice beside the event filter: every open position across accounts, and what holds it | The trader's Positions section, or Futures |

Until a model and an account exist, the page shows the three steps to get started instead.

Each area keeps its size whatever it holds: no problems or many, no positions or forty, one event or two hundred. Lists scroll inside their panel. While the first overview loads, the page already holds the room the scene will take. On a wide window the page fills the first screen and does not scroll.

## Scene

The scene is three-dimensional, drawn with three.js in a chunk only this page loads. Everything runs left to right through four columns named along the front of the floor: data, engines, where they send things, and what comes of it. Calls to a model run on an upper deck and orders and money on the ground, so the routes that would cross pass over each other, and the gates that reach the upper deck stand taller. Each block is a thin standing gate with its members on its face. A route is a curtain whose height follows its last 24 hours on a log scale: a route with nothing is a hairline. It fades in from where it leaves and carries arrowheads, so a still scene still says which way things go. The view starts with the exchanges nearest and the flow running away from them. Drag to turn and tilt it, within limits that keep the whole scene in frame; otherwise it holds still. Ctrl or Command with the wheel, or a pinch, zooms. The button in the corner, or a double click, goes back to where it started.

| Block | One dot is |
| :---- | :--------- |
| Exchanges | The market stream. Hollow when it is down. The figure is what it delivered in the last hour |
| Market | A coin market data follows |
| Telegram | The bound chat |
| Auto trading | A trader. Hollow when stopped |
| Futures | An active plan. Red when it needs reconciliation |
| Forecast | A coin the strategy in use has forecast |
| Models | A host that models are called on, from each model's URL |
| Accounts | An account. Hollow when it cannot be read. The figure is the total equity of those that can |
| Positions | An open position, long before short, coloured by unrealized P/L |

On a phone, or where the browser cannot draw WebGL, the blocks become a list, without routes.

Each event starts with a ring at its source and its member lit, and runs its route once as a comet, in the direction it went. Nothing moves without one. An event that names its coins, such as a closing candle's forecasts, lights those coins in Market and Forecast. Its colour says what it was:

| Colour | Event |
| :----- | :---- |
| Accent | Agent: a call out and back (prompt and reply), or a decision coming back to its trader |
| Primary text | An order, or margin moving into a position |
| Grey | Market data: what the stream delivered each round, a trader's cycle, or forecasts for a closed candle |
| Green | Realized profit returning to an account |
| Red | A loss, a fee, an agent cost, or anything that failed |

A burst of events on one route draws once with a count beside it, and at most 12 lines run at a time. Under reduced motion a route lights up in place instead of travelling, and the view holds still. A model call outside the scene, such as a model or strategy test, is only in the event list.

Hovering a block or route shows its members or its figures for the last hour and day as a tooltip. Clicking one filters the events and dims the rest, and clicking it again or an empty part of the scene clears that. If the stream drops, the page keeps what it has, says so in a corner of the scene and reconnects.

## Data

`GET /api/dashboard/events` serves the events as server-sent events. A feed (`internal/dashboard`) reads the database every 3 seconds and turns new rows into events, so it adds no exchange requests and never touches the trading loop. In the same round it reads the market streams' message counters, and what arrived since the last round goes out as a `stream` message: it runs along the route from Exchanges to Market and counts in that route's totals, but it is not an event, since it would push real events off the list. It keeps the latest 500 in memory, rebuilds from the last 24 hours of the tables when it starts, and keeps exact totals per route. Names come from columns that hold no key, so the feed decrypts nothing.

| Event | Read from | Limits |
| :---- | :-------- | :----- |
| Model call | `agent.calls`, with who made the call and the host it went to. Never the key | Recorded when a call finishes. A failed attempt shows as failed, at no cost, and each retry is its own event |
| Agent cost | `agent.calls` | An estimate from models.dev prices. A call that cannot be priced shows none |
| Decision, market read | `trader.cycles`, `trader.decisions` | Saved when a cycle ends, so times are worked back from its actions |
| Order | `account.orders` | One event per status it is seen in, at the time it got there rather than when the ledger heard, so a sync catching up after an outage is not news. Orders from Futures and orders nothing claims show on their own routes |
| Futures order | `futures.run_orders` | Only an order refused before it reached the exchange, which the ledger never hears of, at the time its row last changed |
| Fee | `account.fills` | A fee not in a dollar stablecoin counts as an event but adds nothing to the route's total |
| Margin, realized P/L | `account.positions` | Every source, Futures included. Leverage comes from the order that opened the position, so a position known only through its fills shows no margin |
| Forecast | `forecast.signals` | Grouped per interval, naming up to 64 of its coins |

The Futures charts read market data directly, without recording it, so there is no route from Market to Futures. Deposits, withdrawals and transfers are not shown: nothing reads them yet.

`GET /api/dashboard/overview` builds the blocks and Positions. It reads balances, positions and open orders from the exchanges, whose request allowance is per IP and shared with live trading, so the result is cached for 15 seconds per user, concurrent requests are merged, and the page polls every 20 seconds only while visible. Balances use the existing 30-second account state cache. Realized today sums positions closed since 00:00 UTC on the user's accounts, whoever traded them: a trader, Futures or a manual close.
