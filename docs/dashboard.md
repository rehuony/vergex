# Dashboard

The Dashboard is a read-only view of the whole system. Nothing on it changes anything: each row opens the page where it can be acted on.

| Area | Shows | Opens |
| :--- | :---- | :---- |
| Needs attention | Blocked or safe-mode traders, traders that could not load, accounts that cannot be read, Frontier plans needing reconciliation, stopped services. Hidden when all is well | The trader, Frontier, or the matching Settings section |
| Metrics | Total equity, traders running, services healthy, positions, open orders, Autopilot P/L today (UTC) | |
| Orchestration | Which AI model each trader asks and which account it trades, Frontier's active plans, and the background services | The trader, Frontier, or Settings |
| Positions | Every open position across accounts, and what holds it | The trader's Positions section, or Frontier |
| Recent orders | The latest orders across traders and Frontier | The trader's Orders section, or Frontier |

Total equity adds up the accounts that could be read. An account that cannot be read appears under Needs attention, and positions and open orders then show a lower bound or a dash rather than zero. If the page stops updating, it keeps the last values and says so.

Open orders counts the orders on symbols with a position or an active plan: the exchange adapters look up open orders one symbol at a time.

## Data

`GET /api/overview` builds all of this. It reads balances, positions and open orders from the exchanges, whose request allowance is per IP and shared with live trading, so the result is cached for 15 seconds per user, concurrent requests are merged, and the page polls every 20 seconds only while visible. Balances use the existing 30-second account state cache.
