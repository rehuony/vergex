# Market data

Trading wants the latest prices with no delay, Forecast needs a lot of history, and Binance limits requests per IP, with live trading sharing the same allowance. So live data arrives over WebSocket streams, candles are stored locally as they close, and REST only starts history and fills holes. Calculations and backtests read only the local store.

```text
Exchange streams (one goroutine per connection)
  │ bounded channel: a full one drops and counts
  ▼
Hub: newest candle per series, last and mark prices, each with when it arrived
  ├─ finished candles ─▶ Writer: batches them into the store, moving the water mark
  │                      a lost or dropped candle leaves the series behind for the hydrator
  └─ followers keep only what is new: Futures charts
```

Nothing downstream can hold the stream up, and every goroutine stops with the application.

## Streams

| Venue | Address | Carries |
| :---- | :------ | :------- |
| Binance | `wss://fstream.binance.com/market/stream` | Every coin's last price and 24-hour open once a second (`!miniTicker@arr`), the watchlist's candles, and what trading and open charts ask for |
| OKX | `wss://ws.okx.com/ws/v5/business` | Candles for open charts; connects only when one is open |
| OKX | `wss://ws.okx.com/ws/v5/public` | Mark prices of OKX positions; connects only while there are any |

Interest in a topic is held by an open chart, or lasts for a time after it was asked for: an hour for a series trading reads or you open, five minutes for a mark price. With nothing to follow, a connection closes.

A connection reconnects with backoff (1 second doubling to a minute) after an error or silence, and Binance's is replaced after 23 hours, before the exchange would close it. Control messages are paced under each exchange's limit. Binance's stream is held to 800 topics, what trading and charts asked for first; anything past that stays on REST.

The top coins are not streamed on purpose. Measured on the live stream, 200 coins at three intervals cost 120 KB/s (10.6 GB a day), against about 3.5 request weight a minute to fetch them after each close. The all-market price stream costs 28 KB/s and gives their prices. The all-market mark price stream, at 166 KB/s, is not used either: only the coins with open positions are followed, at under 1 KB/s each.

The default watchlist with a trader reading three more series measured 42 messages a second and 38 KB/s (3.4 GB a day), about 1% of a CPU core and 43 MB of memory for the whole backend.

## Storage

The market tables are in the main PostgreSQL database, on a connection pool of their own, so bulk candle writes never queue up orders and positions. Everything here can be fetched again, so these tables can be emptied to start over. Forecast's records refer to `market.series`, so emptying that table takes them with it.

| Table | Holds |
| :---- | :---- |
| `market.instruments` | One row per venue, market kind and symbol, so spot and perpetual BTCUSDT would be different instruments. A venue's own name for one is worked out from the symbol when it is needed |
| `market.series` | One row per instrument and interval: its tier, status, water mark, next turn and last error |
| `market.candles` | Closed candles: open, high, low, close, volume and taker buy volume |
| `market.candle_gaps` | Ranges the exchange confirmed it has no candles for |
| `market.derivatives` | Timed funding, open interest value and long/short ratio readings |

The watchlist, the number of high-volume coins and the current ranking are the `market` row of `core.settings`.

## What is collected

Configured in **Forecast → Settings**.

| List | Coins | Intervals | Kept up by |
| :--- | :---- | :-------- | :--------- |
| Watchlist | Chosen by you | 15m, 1h, 4h, 1d | Stream |
| Top coins | Highest 24h volume, 200 by default | 1h, 4h, 1d | REST after each close |
| On demand | A coin you open, or one a trader reads; kept up to date for an hour | Any from 1m to 1d | Stream |

Turning collection off stops the two lists. Whatever trading or you have open keeps working.

The ranking refreshes once a day. A coin enters within the top N and leaves only past N+30, so the sample does not churn. A series that is no longer wanted turns inactive and keeps its data. Candles under 15 minutes are kept for 90 days; the rest are kept for good.

## Filling a series

A series moves forward through its water mark, the newest closed candle that joins the data before it. Both of these move it, and it never goes back.

The **writer** stores finished candles from the stream within a second of the close. It only extends a series from its water mark. A candle that does not join up, or one lost across a reconnect or to a full queue, leaves the series behind, and the series falls due for the hydrator. There is no other recovery path.

The **hydrator** does the rest over REST:

1. **Plan.** Every minute, or right after a settings change, work out which series should exist.
2. **Start.** A new series fetches its latest 300 closed candles (weight 2).
3. **Follow.** After each close plus 3 seconds, fetch from the water mark with `limit` up to 99 (weight 1). A series on the stream has normally moved on by then, so this costs nothing. Candles still open or closed for under 3 seconds are dropped, since one read right at its close can still be missing its last trades, and the rest are validated and written in one transaction with the water mark, which only moves past candles that join up.
4. **Catch up.** More than 99 candles behind, it pages 499 at a time (weight 2), the most candles per unit of weight. A restart carries on from the water mark.
5. **Confirm gaps.** If the next candle is not the expected one, it asks once more, then records the range in `market.candle_gaps` and moves on. A window with a hole that is not recorded is refused rather than calculated on.

Work goes in this order: what you are waiting for, the watchlist's latest candles (shorter intervals first), the top coins' latest candles, new series, long catch-ups.

## What trading reads

`market.Klines`, which feeds the prompts and the price behind order sizing and stop-loss checks, asks for the latest candles of a series. For a Binance series, the answer is local history plus the candle forming on the stream, only when that can be vouched for: the stream is up, the forming candle is the current one, and history joins up with it. Anything else gets the exchange's own answer. Reading also opens the series, so the next read is local.

OKX traders read OKX's candles directly over REST, which is cheap and always current, and Binance's if OKX has none.

The drawdown monitor reads positions from the exchange every minute, as the check of record. Between reads, the live mark price keeps each position's peak current and can call that check within a second of a giveback. It never closes anything itself, and if the stream stops the minute check carries on alone.

The Futures chart gets what the exchange has when it opens, then the stream, and polls every 3 seconds while no stream is up. Plan snapshots and execution do not read any of this.

A [testnet](accounts.md#testnet) account changes none of this: all of it stays on the real market. Only the account's own balance, positions, orders and fills, and the book a Futures plan on it reads, come from the test environment.

## Rate limits

| Protection | Behaviour |
| :--------- | :-------- |
| Budget | 300 weight per minute for candles and prices (12.5% of the 2400 Binance allows), 150 requests per minute for `/futures/data/*`, 60 for funding history, and a separate 600 weight per minute for reads a running trader is waiting on, so a long catch-up cannot queue them |
| Used weight | Reads `X-MBX-USED-WEIGHT-1M`, which includes live trading on this IP, and above 1200 pauses background requests to the next minute. Trading's reads carry on |
| 429, 403 | Pauses everything, using `Retry-After` or else 60 seconds doubling up to 30 minutes |
| 418 | Pauses everything for `Retry-After` or at least an hour, and the Dashboard shows the service as failed |
| Public reads | The order books, chart history and data plugins Futures reads don't wait for any budget, so a long catch-up cannot hold up someone planning a trade. Their responses still reach the limiter, so the weight they use and any 429 or 418 pause background requests |
| 5xx, timeouts | Backs off that series only |
| Retries | None within a request; the series goes back in the queue |
| Silent connections | Every REST request to a venue, trading included, goes through `service.VenueTransport`. A connection that has received nothing for 10 seconds gets an HTTP/2 ping, and is closed if no answer comes within 5. That fails the requests waiting on it, and the next one opens a new connection |

## Derivatives

Open interest and long/short ratio are saved every 15 minutes for the watchlist and hourly for top coins. Funding is checked hourly. Binance keeps open interest and long/short for only about 30 days over its API, so collecting them is what builds their history. Older history comes from the files below.

## Deep history

Binance publishes history as files at `data.binance.vision`, which cost no API weight. Each file's checksum is verified and it is read in memory, never unpacked to disk. What the files do not cover yet, up to the recent stretch, is read over REST within the budget.

```bash
vergex market-backfill --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24
```

`--derivatives` also loads funding (monthly files) and hourly open interest and long/short ratio (one file per day, so slower). The same download can be started from Forecast → Backtests, where local history is short. It only runs when asked.
