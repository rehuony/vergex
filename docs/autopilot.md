# Autopilot

An Autopilot trader runs a strategy on a schedule: it reads the market, asks the model for decisions and places orders within the strategy's limits. Traders and strategies are managed on the **Autopilot** page, under the **Traders** and **Strategies** tabs.

A trader binds an AI model, an exchange account and a strategy.

The Traders tab lists each trader with its stored equity, return, win rate and drawdown, read from the database, and **Stop all** stops every running trader without closing positions. A row opens the trader's own page, which is the only place to start, stop, edit or delete it:

| Section | Shows |
| :------ | :---- |
| Performance | Equity, P/L, exposure, the equity curve (from the snapshot each cycle stores), results by symbol, hold time and side |
| Positions | Open positions, closing one or all of them, and recent closes |
| Decisions | The latest cycle, and a log of every cycle. Opening one shows its prompts, reasoning and raw response |
| Orders | Recorded orders and their fills, with CSV export |

Starting a trader opens a confirmation with what will run and the launch checks: AI model, strategy, exchange account, account connection and available balance. It cannot be started while a check has failed.

The Strategies page edits every strategy. AI strategies have Coins, Risk, Market data and Prompt sections; grid strategies have one form. A strategy's type is set when it is created and cannot change. Saved changes reach running traders on their next cycle.

Test tries an AI strategy as it stands in the form, saved or not, on live public market data. **Preview prompts** only builds the prompts. **Run test** also sends them to a model you choose, on your API key and at your cost. Neither places orders.

## Cycle

`runCycle` in `internal/trader/auto_trader_loop.go` runs every `scan_interval_minutes` (15 by default):

1. Check the running state, reload the strategy if it changed, and check risk pauses and safe mode.
2. Build the context: account, open positions, candidate coins.
3. Fetch market data, build the prompts, call the model, and parse and validate the decisions (`internal/kernel/`).
4. Drop decisions outside the strategy's coins, execute closes before opens, and store the decision record, AI usage and an equity snapshot.

## Candidate coins

Set by `coin_source.source_type`:

| Source | Meaning |
| :----- | :------ |
| `static` | The configured `static_coins` |
| `volume_top` | Highest 24h quote volume |
| `gainers`, `losers` | Largest 24h rise or fall, above `min_quote_volume` |
| `oi_top`, `oi_low` | Largest open-interest increase or decrease over `oi_window` (1h, 4h, 24h) among the `oi_pool_size` highest-volume symbols |
| `mixed` | Union of the enabled ranked sources plus the static coins |

Rankings come from Binance public data and are shared by all traders (tickers cached for 1 minute, open interest for 15). Excluded coins never take a slot. An unknown source type falls back to `static`. Each coin's ranking data is passed to the prompt.

## Market data

K-lines come from CoinAnk with a Binance fallback, for the configured timeframes and bar count. Indicators are opt-in: EMA, MACD, RSI, ATR and Bollinger bands are off by default; volume, open interest and funding rate are on. Coins without market data or with too little open interest are dropped, while coins you already hold are always kept.

## Prompts

The system prompt (`internal/kernel/engine_prompt_system.go`) is always English, so model behaviour stays stable. It is assembled in this order:

1. Data dictionary, role definition and trading mode.
2. Hard constraints from `risk_control`.
3. Trading frequency, entry standards and decision process.
4. The JSON output contract.
5. Your `custom_prompt`.

The Strategies page edits `custom_prompt` and the sections in `prompt_sections` (`role_definition`, `trading_frequency`, `entry_standards`, `decision_process`).

Writing a custom prompt:

- Describe your strategy: what to trade, when to enter, when to stay out. The market data and output format are added for you.
- Write it in English. A strategy whose `language` is `zh` ignores its stored prompt sections and custom prompt.
- It supplements the rules and cannot override the risk limits below.
- Don't ask for fields the data doesn't contain, or for a different output format.

## Limits

These bounds are applied whenever a strategy is loaded (`internal/store/strategy_normalize.go`):

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
| Candidate coins | | at most 10 per source |
| Timeframes, bars | 3 timeframes, 20 bars | at most 4, 10–30 bars |

Position count, position value, margin usage and minimum size are enforced in code. Leverage, risk/reward and confidence are given to the model as rules, and decisions are validated before execution.

### Token budget

`StrategyConfig.EstimateTokens` (`internal/store/strategy_tokens.go`) estimates the prompt size from coins, timeframes, bars and indicators with a 15% margin. It warns at 80% of the model's context window and flags 100% as a danger. The limits above keep the default setup well inside every provider's window; the largest setup (4 timeframes, 30 bars, all indicators) is the one to watch.

## Execution and risk control

- **Opens** enforce the limits above, set leverage and margin mode, place the order, then place stop-loss and take-profit. If protection cannot be placed, the position is closed.
- **Hold times** apply to AI-initiated closes (`auto_trader_throttle.go`). A position is held for at least 90 minutes unless price has moved past -3% or +8%. Between 90 minutes and 3 hours, a close while price is between -2% and +3% also waits.
- **Re-entry** into a just-closed symbol waits 4 hours.
- **Drawdown monitor** (`auto_trader_risk.go`): once price has moved 5% in the position's favour, it closes the position if it gives back 40% of its peak profit.
- **Safe mode**: after 3 consecutive AI failures the trader opens no new positions until a call succeeds. Existing positions keep their protection.

An exchange account leased by an unresolved [Frontier](frontier.md) plan cannot start an Autopilot trader.

## Grid strategies

`strategy_type: grid_trading` uses `grid_config` and the grid engine (`internal/kernel/grid_engine.go`, `internal/trader/auto_trader_grid*.go`). The model manages grid levels for one symbol, within your budget, leverage and bounds, which can come from ATR.

Regime checks use three Donchian boxes on 1h candles: short (72 bars), mid (240) and long (500). A breakout must hold for 3 candles to count:

| Breakout | Action |
| :------- | :----- |
| Short box | Reduce position to 50% |
| Mid box | Pause the grid and cancel its orders |
| Long box | Pause, cancel and close all positions |

With `enable_direction_adjust`, breakouts can instead shift the grid to a long or short bias (`direction_bias_ratio`). `max_drawdown_pct`, `daily_loss_limit_pct` and `stop_loss_pct` act as hard stops.
