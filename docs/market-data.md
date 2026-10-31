# Market data

Forecast needs a lot of candles, and Binance limits requests per IP, with live trading sharing the same allowance. So candles are stored locally and fetched incrementally in the background. Calculations and backtests read only the local store; only the hydrator asks the exchange.

## Storage

SQLite deployments keep this in its own `market.db` beside the main database, using WAL and `synchronous=NORMAL`, so bulk candle writes never queue up orders and positions. PostgreSQL deployments use the same database through a separate connection pool. Everything here can be fetched again, so it is safe to delete `market.db` to start over.

| Table | Holds |
| :---- | :---- |
| `market_series` | One row per symbol and interval: its tier, status, water mark, next turn and last error |
| `market_candles` | Closed candles |
| `market_gaps` | Ranges the exchange confirmed it has no candles for |
| `market_derivatives` | Timed funding, open interest value and long/short ratio readings |
| `market_settings` | The watchlist, the number of high-volume coins and the current ranking |

## What is collected

Configured in **Settings → Forecast**.

| List | Coins | Intervals |
| :--- | :---- | :-------- |
| Watchlist | Chosen by you | 15m, 1h, 4h, 1d |
| Top coins | Highest 24h volume, 200 by default | 1h, 4h, 1d |
| On demand | A coin you open; kept up to date for an hour | The interval you open |

The ranking refreshes once a day. A coin enters within the top N and leaves only past N+30, so the sample does not churn. A series that is no longer wanted turns inactive and keeps its data.

## Hydration

1. **Plan.** Every minute, or right after a settings change, work out which series should exist.
2. **Start.** A new series fetches its latest 300 closed candles (weight 2).
3. **Follow.** After each close plus 3 seconds, fetch from the water mark with `limit` up to 99 (weight 1). Candles still open are dropped, and the rest are validated and written in one transaction with the water mark, which only moves past candles that join up.
4. **Catch up.** More than 99 candles behind, it pages 499 at a time (weight 2), the most candles per unit of weight. A restart carries on from the water mark.
5. **Confirm gaps.** If the next candle is not the expected one, it asks once more, then records the range in `market_gaps` and moves on. A window with a hole that is not recorded is refused rather than calculated on.

Work goes in this order: what you are waiting for, the watchlist's latest candles (shorter intervals first), the top coins' latest candles, new series, long catch-ups.

## Rate limits

| Protection | Behaviour |
| :--------- | :-------- |
| Budget | 300 weight per minute for candles and prices (12.5% of the 2400 Binance allows), 150 requests per minute for `/futures/data/*`, 60 for funding history |
| Used weight | Reads `X-MBX-USED-WEIGHT-1M`, which includes live trading on this IP, and pauses to the next minute above 1200 |
| 429, 403 | Pauses everything, using `Retry-After` or else 60 seconds doubling up to 30 minutes |
| 418 | Pauses everything for `Retry-After` or at least an hour, and the Dashboard shows the service as failed |
| 5xx, timeouts | Backs off that series only |
| Retries | None within a request; the series goes back in the queue |

## Derivatives

Open interest and long/short ratio are saved every 15 minutes for the watchlist and hourly for top coins. Funding is checked hourly. Binance keeps open interest and long/short for only about 30 days over its API, so collecting them is what builds their history. Older history comes from the files below.

## Deep history

Binance publishes history as files at `data.binance.vision`, which cost no API weight. Each file's checksum is verified and it is read in memory, never unpacked to disk. What the files do not cover yet, up to the recent stretch, is read over REST within the budget.

```bash
vergex market-backfill --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24
```

`--derivatives` also loads funding (monthly files) and hourly open interest and long/short ratio (one file per day, so slower). The same download can be started from Forecast → Backtests, where local history is short. It only runs when asked.
