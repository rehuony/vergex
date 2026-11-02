# Auto trading

Auto trading, shown in Chinese as `自动交易`, is made of traders (`交易员`). A trader runs on a schedule: it reads the market, asks the model for decisions and places orders within the limits in its settings. It binds a model and an account, and carries its own settings: a kind, fixed when it is created, and a config that belongs to that kind. There are two kinds, `agent` and `grid`.

The **Traders** tab lists each trader, running ones first, with its kind, model and stored return, and opens one beside the list in place. From 8 traders on a search box filters by name. The address says what is open, as `/traders?id=&tab=`, and without it the page opens the last one looked at. **Overview** adds up the traders and compares them in a table a column header sorts: how many run, the equity of the accounts they use, counting an account once however many traders share it and leaving [testnet](accounts.md#testnet) accounts out, and each trader's account equity, return, win rate, drawdown and trades, read from the database. Equity is the whole account's, in the list and on the curve under Performance. Return and total P/L are the trader's own, from the ledger: what it realized after fees and what its open positions stand at, against the balance it started with. Other traders on the account, Futures and transfers don't touch them. **Stop all** stops every running trader without closing positions. Until both a model and an account exist, the page says what is missing and links to it in Settings.

**Create trader** asks for the name, kind, a preset, the model, the account, the scan interval and the margin mode, then opens the new trader. The presets are starting points written in code: `careful`, `balanced` and `active` for an agent trader, one `default` for a grid. Nothing is seeded when you register.

A trader is started and stopped from its header, and what else can be done to it is in the list's `…` menu, or the header's on a screen too narrow for the list: renaming, duplicating and deleting. Deleting one stops it and archives it: it disappears everywhere, while its cycles, equity, conversations and ledger entries stay in the database. Its model and account link to where they are edited, and cannot be deleted while a trader uses them; an archived trader no longer does. Beside its name is how it stands, and trading blocked by the exchange or safe mode takes that place and opens in full:

| Tab | Shows |
| :-- | :---- |
| Performance | Account equity, P/L, exposure, the account's equity curve (from the snapshot each cycle stores), results by symbol, hold time and side, and what its model calls have cost, with what it realized net of fees and that cost. A read that fails keeps the last values and says they are not updating |
| Positions | Open positions, closing one or all of them, and closed trades, with older ones on request |
| Decisions | The latest cycle, and a log of every cycle. Opening one shows what its model call cost, its prompts, reasoning and raw response |
| Orders | Recorded orders and their fills, with CSV export |
| Settings | What it runs with under Basics: its model, account, interval and margin mode. Then its config: Coins, Risk, Market data and Prompt parts for an agent trader, one form for a grid |

A name edits where it stands, in the list or in the header, and Basics can be changed, while the trader is stopped: changing what a running trader runs with restarts it. **Duplicate** copies a trader, stopped, with its settings.

A saved settings change reaches a running agent trader on its next cycle, and restarts a running grid trader, since its levels are built from those settings. The form says which will happen before you save. A draft stays while you move between tabs, and leaving for another trader asks first.

**Dry run** tries the settings as they stand in the form, saved or not, on live public market data. **Preview prompts** only builds the prompts. **Run test** also sends them to a model you choose, on your API key and at your cost. Neither places orders.

Starting a trader opens a confirmation with what will run and the launch checks: model, settings, account, account connection and available balance. It cannot be started while a check has failed.

## Cycle

`runCycle` in `internal/trader/auto_trader_loop.go` runs every `scan_interval_minutes` (15 by default):

1. Check the running state and reload the settings if they changed.
2. Build the context: account, open positions, candidate coins, and store an equity snapshot. Check the loss limits: a [halted](#halts) trader holding nothing stops here.
3. Fetch market data, build the prompts, call the model, and parse and validate the decisions (`internal/trader/agent/`).
4. Drop decisions outside the trader's coins, and opens while halted, execute closes before opens, and store the cycle and agent usage.

A cycle is a row in `trader.cycles`, with the limits it ran under in `limits` and what the model decided in `trader.decisions`, each pointing at the order it became. What was sent to the model and what it said, with its own thinking where the provider returns it and otherwise the reasoning in its reply, are a conversation in `agent.messages`; a call that failed keeps what was sent.

Keeping these records never costs the loop: opening the conversation waits 3 seconds at most, its messages are written in the background, and saving the cycle waits 5 seconds at most. A failure is only logged, and that record is lost. Nothing cleans them up: see [Data and backups](getting-started.md#data-and-backups).

## Candidate coins

Set by `coin_source.source_type`:

| Source | Meaning |
| :----- | :------ |
| `static` | The configured `static_coins` |
| `volume_top` | Highest 24h quote volume |
| `gainers`, `losers` | Largest 24h rise or fall, above `min_quote_volume` |
| `oi_top`, `oi_low` | Largest open-interest increase or decrease over `oi_window` (1h, 4h, 24h) among the `oi_pool_size` highest-volume symbols |
| `mixed` | Union of the enabled ranked sources plus the static coins |

Rankings come from Binance public data and are shared by all traders (tickers cached for 1 minute, open interest for 15). Excluded coins never take a slot. A source type outside this table is refused when the settings are saved. Each coin's ranking data is passed to the prompt.

## Market data

K-lines come from local history and the live stream, or from the exchange when those cannot be vouched for ([market-data.md](market-data.md#what-trading-reads)), for the configured timeframes and bar count. Indicators are opt-in: EMA, MACD, RSI, ATR and Bollinger bands are off by default; volume, open interest and funding rate are on. Coins without market data or with too little open interest are dropped, while coins you already hold are always kept.

## Prompts

The system prompt (`internal/trader/agent/engine_prompt_system.go`) is always English, so model behaviour stays stable. It is assembled in this order:

1. Data dictionary, role definition and trading mode.
2. Hard constraints from `risk_control`.
3. Trading frequency, entry standards and decision process.
4. The JSON output contract.
5. Your `custom_prompt`.

The Prompt part of Settings edits `custom_prompt` and the sections in `prompt_sections` (`role_definition`, `trading_frequency`, `entry_standards`, `decision_process`).

Writing a custom prompt:

- Describe how to trade: what to trade, when to enter, when to stay out. The market data and output format are added for you.
- Write it in English. A prompt section written in Chinese is ignored, since system prompts are English only.
- It supplements the rules and cannot override the risk limits below.
- Don't ask for fields the data doesn't contain, or for a different output format.

## Limits

These bounds are applied whenever settings are saved or loaded (`Validate` in `internal/trader/kind/agent.go`):

| Setting | Default | Range |
| :------ | :------ | :---- |
| Max positions | 3 | 1–8 |
| BTC/ETH leverage | 5x | 1–20x |
| Altcoin leverage | 5x | 1–20x |
| BTC/ETH position value | 5× equity | 0.5–10× |
| Altcoin position value | 1× equity | 0.5–10× |
| Margin usage | 90% | 10–100% |
| Min position size | 12 USDT | 10–1000 USDT |
| Min risk/reward | 3:1 | 1–10 |
| Min confidence | 75 | 50–100 |
| Max daily loss | 5% | 0 is off, 0.5–100% |
| Max drawdown | 15% | 0 is off, 0.5–100% |
| Candidate coins | | at most 10 per source |
| Timeframes, bars | 3 timeframes, 20 bars | at most 4, 10–30 bars |

The model is told all of these, and code holds it to them just before an order. Leverage above its limit is lowered to it, and a position too large for the margin left under the usage limit is cut down. An open is refused if it is over its value limit, if it would then be under the minimum size, if it would pass the position count, if its confidence is too low, or if its reward to risk, worked out from the current price, is too low.

A reply cut off at its length limit before it could be read says so, rather than only that it could not be parsed. It counts as a failed model call, and `VERGEX_AGENT_MAX_TOKENS` raises the limit.

A refused decision is only that decision: the rest of the reply still happens, including closes, it shows as blocked in the Decisions log, and it raises no alert and does not count towards safe mode.

### Token budget

`EstimateTokens` (`internal/trader/kind/tokens.go`) estimates the prompt size from coins, timeframes, bars and indicators with a 15% margin. It warns at 80% of the model's context window and flags 100% as a danger. The limits above keep the default setup well inside every provider's window; the largest setup (4 timeframes, 30 bars, all indicators) is the one to watch.

## Execution and risk control

- **Opens** enforce the limits above, set leverage and margin mode, place the order, then place stop-loss and take-profit. If protection cannot be placed, the position is closed.
- **Hold times** apply to agent-initiated closes (`auto_trader_throttle.go`). A position is held for at least 90 minutes unless price has moved past -3% or +8%. Between 90 minutes and 3 hours, a close while price is between -2% and +3% also waits.
- **Re-entry** into a just-closed symbol waits 4 hours.
- **Drawdown monitor** (`auto_trader_risk.go`): once price has moved 5% in the position's favour, it closes the position if it gives back 40% of its peak profit. It reads positions every minute and follows the live mark price between reads ([market-data.md](market-data.md#what-trading-reads)). A position's peak is stored, so a restart does not forget what has been given back.
- **Loss limits**: losing `max_daily_loss_pct` of equity since 00:00 UTC [halts](#halts) the trader until the next UTC day, and equity `max_drawdown_pct` below its peak halts it until you resume it. A halted agent trader opens nothing. It keeps asking the model about what it holds, so a position can still be closed, and its stops stay. Equity is the whole account's, so another trader's loss or a withdrawal counts.
- **Safe mode**: after 3 consecutive model failures the trader opens no new positions until a call succeeds. Existing positions keep their protection.

An account leased by an unresolved [Futures](futures.md) plan cannot start a trader.

A trader reads the whole account, so it treats every position there as its own: one you opened by hand counts towards its position limit, reaches its model, can be closed by the model or the drawdown monitor. Starting it lists the positions the ledger does not give to it, and its Positions tab marks them. Keep a trader on an account of its own if that matters. A new position can read as not its own until its fill is booked, within 30 seconds.

## Orders, fills and positions

`internal/account/syncer` runs one loop per account while something can be trading on it: a running trader, a reserved Futures trade, or a position still open in the ledger, and for ten minutes after that. Every 30 seconds it reads the account's new trades and books each one in a single transaction (`Ledger.RecordTrade` in `internal/account`), so reporting a trade twice changes nothing.

- **What gets recorded when.** A trader records an agent open or close, and a grid limit order, right after placing it, so its row has the real leverage and client order ID. Its stops, targets, drawdown closes and manual closes are not recorded when placed, because those calls return no order: their row appears when they fill. Orders placed on the exchange by hand appear the same way.
- **Settling.** While the ledger has an order as working, the loop lists that symbol's open orders, and asks about any it no longer finds, so a cancel or a rejection, which leaves no fill, is recorded too. It only makes these requests when such an order exists, and follows one up for seven days. Futures orders are left out: a run follows its own and tells the ledger how they ended.
- **Owners.** An order nothing knows of goes to whoever holds the position it closes, then to whoever has reserved the account, then to a trader running on it, and otherwise is `external`. So a stop that fires after its trader stopped still lands on that trader's position.
- **Closes.** A close reduces its own source's position first, then the oldest. Two traders on one account share one exchange position, so which of them a stop belongs to is inferred, and can be wrong in hedge mode.
- **One-way mode.** A trade reduces what is open against it and opens the rest, so a reversal becomes a close and an open.
- **Out of order.** A close with no position to reduce waits on its fill (`unmatched_quantity`) for an older open to arrive. One older than every open stays there, because what it closed predates the ledger.
- **Fees.** The real fee asset is kept on the fill. Only fees in dollar stablecoins add up on the position.
- **After an outage.** Reading starts from the last known trade however long ago that was, and only moves on once a whole read has worked. Both exchanges keep three months of trades, so anything older cannot be recovered, and the log says so. Binance reads a week at a time and asks about every symbol the ledger still has open, which is how a stop that fired during an outage is found.
- **A failed read on OKX.** Fills are read across all symbols, newest first, paging back to the start of the read. Their sizes are in contracts, so when a contract's value cannot be read right now, the whole read fails and is tried again, rather than being booked at the wrong size.
- **A trade that cannot be booked.** When the answer will not change, such as a contract OKX no longer describes, the trade goes to `account.skipped_fills` and the rest carry on. It is not in the ledger, so a position it closed stays open there. **Needs attention** on the Dashboard names the contract, the trade and the size for 30 days.
- **Numbers.** Everything is an exact decimal. The exchange's realized P/L wins, and it is only worked out from the entry price when the exchange reports none.

An account with any orders, fills or positions cannot be deleted, so its history stays whole.

## Grid traders

A trader of kind `grid` uses the grid engine (`internal/trader/agent/grid_engine.go`, `internal/trader/auto_trader_grid*.go`, and the pure calculations in `internal/trader/grid`). The model manages grid levels for one symbol, within your budget, leverage and bounds, which can come from ATR.

Regime checks use three Donchian boxes on 1h candles: short (72 bars), mid (240) and long (500). A breakout must hold for 3 candles to count:

| Breakout | Action |
| :------- | :----- |
| Short box | Reduce position to 50% |
| Mid box | Pause the grid and cancel its orders |
| Long box | Pause, cancel and close all positions |

With `enable_direction_adjust`, breakouts can instead shift the grid to a long or short bias (`direction_bias_ratio`).

Three hard stops apply, and each cycle checks them before anything else:

| Setting | Measures | Then |
| :------ | :------- | :--- |
| `max_drawdown_pct` | The account's equity against its peak since the trader was last resumed | Cancels the grid's orders, closes its positions and [halts](#halts) until you resume it |
| `daily_loss_limit_pct` | What the ledger has realized for this trader today (UTC), against `total_investment` | Cancels its orders and halts until the next UTC day. Positions stay |
| `stop_loss_pct` | One filled level's loss against its entry | Closes that level |

Equity is the whole account's, so a withdrawal counts as a loss.

Stopping a grid trader cancels every order on its symbol, since nothing would watch what they open, and says so if it could not. Its positions stay. Starting one clears what a crash left behind in the same way, so keep other orders on that symbol on another account.

The trader's page shows the grid's risk (leverage, position, regime and boxes) only while it runs. A stopped trader has no reading, even with a position still open on the exchange, which the Positions tab shows.

## Halts

A hard stop that trips halts the trader. The halt is stored with it, so a restart or a change of settings does not start trading again. It shows beside the trader's name and under **Needs attention** on the Dashboard, with the reason. **Resume**, in what opens from there, ends it, and peak equity starts over from that moment. A halt with an end time ends by itself then.
