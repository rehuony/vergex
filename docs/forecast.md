# Forecast

Forecast scores where a coin may go next from local market data, records each call and checks it later. It is an experiment to find out whether such signals have any value. It is not a proven strategy, nothing trades on it, and the numbers below say so.

Open **Forecast** in the top bar. Data comes from [market data](market-data.md), so a page or a backtest never adds exchange requests, apart from a price read when the live stream is down.

## Pages

| Page | Shows |
| :--- | :---- |
| Signals | The chart of one coin with its call beside it and every tracked coin to move between. `/forecast` opens the last coin looked at, and `/forecast/:symbol` a given one. Factors and the record are below the first screen |
| Strategies | Saved strategies, and the form to make a new one from any of them |
| Backtests | Test a strategy on local history, and compare runs |
| Settings | What is collected, the alert threshold, and how collection and the live stream are doing |

The strategy in use is chosen beside the tabs. Choosing one puts it to use: signals and alerts follow it, and nothing trades on them.

## The chart

The chart follows the live stream, so it spends no requests. The **Chart** menu turns on or off, remembered on this browser:

| Option | Draws |
| :----- | :---- |
| Support and resistance | The levels from the reading |
| Past calls | A mark at each judged call: green right, red wrong |
| Simulated candles | The forecast as candles over its horizon |
| EMA 20, 50; Bollinger 20, 2; VWAP 24; RSI 14 | The series behind the factors, worked out the same way as a forecast at each candle, and fetched only when one is on. RSI has a pane of its own |

Simulated candles restate the reading and add no information. From the score and expected move they set how far the path goes, each spanning one typical bar's range, with no randomness and nothing stored. Only the direction of these calls has been tested, not the size or the path, so they are drawn see-through with a note saying they are not prices.

The reading shows this strategy's hit rate on the coin next to how often price rose anyway over the same records. A hit rate means nothing until it beats that.

Frontier's chart can overlay the same simulated candles and reading: see [Frontier](frontier.md).

The coin list orders strong calls first and keeps rows in place while values refresh, so nothing moves under the pointer: it reorders only on a new interval or sort. Typing a coin that is not tracked and pressing Enter opens it and starts fetching it.

## How a forecast is made

`forecast.Evaluate` is a pure function of a strategy and the data up to a candle: the last 200 candles and the derivatives readings at that moment. Live forecasts and backtests call it the same way, so a candle gets the same answer either way, and nothing after that candle can affect it.

1. **Factors.** Each reads from -1 (points down) to 1 (points up): trend, momentum, RSI, Bollinger position, recent returns, volume, candle shape, swing structure, stochastic, VWAP distance, divergence, funding rate, open interest and long/short ratio.
2. **Market.** ADX splits trending from ranging markets, and each has its own weights. A negative weight reads a factor the other way round: an overbought RSI supports a trend but argues against a range.
3. **Score.** The weighted average goes through a logistic curve. It is calculated twice: with candles only and with derivatives added. A factor with no recent reading is left out.
4. **Call.** Up or Down when the score is far enough from 50, otherwise Neutral. Confidence falls when factors are missing. Expected move comes from ATR over the horizon, and support and resistance from recent swing points.

A coin with too little history shows `Not enough history`. One whose candles have an unexplained hole is skipped until the hole is filled.

## Score and probability

A raw score is not a probability, so it shows as `Score 68`. Once a strategy and interval have 200 non-overlapping verified forecasts, isotonic regression maps scores to what actually happened, and the display changes to `68%`. The curve is rebuilt hourly from verified live forecasts only and applies to forecasts made after it.

## Checking the calls

Every closed candle of a tracked series gets one record per strategy in use. Forecasts run up to `horizon` bars ahead, 4 by default. When the target candle closes, the call is judged against its close: `Hit` if the direction was right, `Miss` if it was wrong, and `Not judged` if it was Neutral or price did not move. The actual move is kept either way.

## Strategies

A saved strategy never changes, so its records always describe one set of rules. To try something else, change the form from any strategy and save it under a new name, backtest it, then choose it beside the tabs. A strategy not in use can be deleted, with its records and backtests. `EngineVersion`, an internal field, goes up when factor code changes, so results from different code are not mistaken for each other.

The config carries a `scorer` field, only `weighted` so far. Another way of scoring, including a model, can use the same strategies, records, verification and backtests.

## Backtests

A backtest reads only local data, one at a time, in the background. Where local history is short, the form says so and offers to download it. See [deep history](market-data.md#deep-history).

```bash
vergex forecast-backtest --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24
```

| Metric | Meaning |
| :----- | :------ |
| Samples | Non-overlapping tests: each series every `horizon` bars, so one move is not counted several times |
| Hit rate | Hits over directional calls that moved |
| Rose anyway | How often price simply rose, over the samples where it moved. This is what a hit rate has to beat |
| Brier score, lift | Error of the scores, and the improvement over always predicting the base rate. Zero or below means no skill beyond it |
| Per signal after fees | Average return of acting on each call, less 0.10% round-trip fees |
| Within expected move | How often the actual move stayed inside the expected move |
| Reliability | For each range of scores, what was said against how often price rose |

### First results

The default weights are a first guess, not a tested result. On BTC, ETH and SOL over 24 months at 1h, 4h and 1d, with candles only:

| | Samples | Hit rate | Rose anyway | Lift | Per signal after fees |
| :- | --: | --: | --: | --: | --: |
| Overall | 16,818 | 49.9% | 50.5% | -17.4% | -0.109% |
| Ranging | 8,545 | 51.6% | 50.6% | -9.5% | -0.139% |
| Trending | 8,273 | 48.4% | 50.5% | -25.7% | -0.082% |

The reliability table was flat: price rose about half the time at every score. So the default strategy has shown no predictive value. Its scores are also overconfident, which is why they are not shown as probabilities. Treat the signals as an experiment until a version does better on data it was not tuned on.

## Alerts

Watchlist coins alert over Telegram when a score crosses the threshold in **Forecast → Settings**: at 70, that means 70 and above or 30 and below. A coin alerts once, checked only when a candle closes, and can alert again after its score comes back inside. Turn it off with Forecast signals in **Settings → Telegram**.
