# Forecast

Forecast scores where a coin may go next from local market data, records each call and checks it later. It is an experiment to find out whether such signals have any value. It is not a proven strategy, nothing trades on it, and the numbers below say so.

Open **Forecast** in the top bar. Data comes from [market data](market-data.md), so a page or a backtest never adds exchange requests, apart from a price read when the live stream is down.

## Pages

| Page | Shows |
| :--- | :---- |
| Signals | Every tracked coin in a list that stays beside the page, then one coin's chart filling the screen with its call at the head, and what the call rests on, its factors and its record below. `/forecast` opens the last coin looked at, and `/forecast?symbol=BTCUSDT&interval=1h` a given one |
| Strategies | Saved strategies in a list that stays beside the page, then how one has done live and its rules |
| Backtests | Past runs in a list that stays beside the page, then one run's report, or the form to start another |
| Settings | What is collected, saved with its button, and the alert threshold, saved on leaving its field, beside how collection and the live stream are doing |

The strategy in use is chosen from the picker on Signals, or with **Use** on Strategies. Choosing one puts it to use: signals and alerts follow it, and nothing trades on them.

The page remembers the last coin and interval, and the address says which coin, strategy or run is open, so a reload stays on it and shows what it last read while it reads again. Resting the pointer on a coin reads it ahead, so opening it shows it at once. Moving between coins adds nothing to the browser's history, so Back leaves Forecast.

## The chart

The chart follows the live stream, so it spends no requests.

| To | Do |
| :-- | :-- |
| Scroll the page | Scroll as usual, even with the pointer over the chart |
| Move along time | Drag, or scroll sideways |
| Zoom | Hold Ctrl or Command and scroll, pinch, or use the buttons that show when the pointer is over the chart |
| Go back to the latest | The last of those buttons |

A line at the top of the chart gives the open, high, low, close, change and volume of the candle under the pointer, or of the newest one.

The **Chart** menu turns on or off, remembered on this browser:

| Option | Draws |
| :----- | :---- |
| Support and resistance | The levels from the reading |
| Past calls | A mark at each judged call: green right, red wrong |
| Simulated candles | The forecast as candles over its horizon |
| EMA 20, 50; Bollinger 20, 2; VWAP 24; RSI 14 | The series behind the factors, worked out the same way as a forecast at each candle, and fetched only when one is on. RSI has a pane of its own |

Simulated candles restate the reading and add no information. From the score and expected move they set how far the path goes, each spanning one typical bar's range, with no randomness and nothing stored. They run flat on a neutral call, and where a calibrated score sits on the other side of 50 from the call. Only the direction of these calls has been tested, not the size or the path, so they are drawn see-through.

The reading shows this strategy's hit rate on the coin next to what the same mix of Up and Down calls would hit by chance, given how often price rose. A hit rate means nothing until it beats that: always calling Down in a falling market hits often, with no skill. Both count only forecasts whose windows do not overlap, the same way a backtest does.

The Futures chart can overlay the same simulated candles and reading: see [Futures](futures.md).

The coin list orders strong calls first and keeps rows in place while values refresh, so nothing moves under the pointer: it reorders only on a new interval or sort. It shows each coin's price, 24-hour change and score, with the change blank while the live stream is down, and can be narrowed to the watchlist, to Up calls or to Down calls. Typing a coin that is not tracked and pressing Enter opens it and starts fetching it. In the search box, the up and down keys move through the list, Enter opens the row they are on, and Escape clears the search.

The star beside a coin puts it on the watchlist, shown in Chinese as `收藏`, or takes it off. A starred coin is collected at every interval and can alert, so it keeps a forecast whatever its volume does. The watchlist holds up to 100 coins.

The chart's head gives the price, the 24-hour change, the call and its score. The panel under the chart gives the confidence, the expected move, when the call will be judged, support and resistance with their distance from the price, the market and its ADX, the hit rate with its sample size, what chance would hit, the funding rate, the open interest change, the long/short ratio against its own recent range, and when the forecast was made. It marks a series that has fallen behind as stale and keeps its last values. Factors lists each factor's reading, weight and effect on the score, and those with no reading.

## How a forecast is made

`forecast.Evaluate` is a pure function of a strategy and the data up to a candle: the last 200 candles, and the derivatives readings as of the candle's close less one hour, since a reading from the close itself only arrives after it. Live forecasts and backtests call it the same way, so a candle gets the same answer either way, and nothing after that candle can affect it. The exception is a window across a gap Binance confirmed it has no candles for: live forecasts accept it, and a backtest skips it.

1. **Factors.** Each reads from -1 (points down) to 1 (points up): trend, momentum, RSI, Bollinger position, recent returns, volume, candle shape, swing structure, stochastic, VWAP distance, divergence, funding rate, open interest and long/short ratio.
2. **Market.** ADX splits trending from ranging markets, and each has its own weights. A negative weight reads a factor the other way round: an overbought RSI supports a trend but argues against a range.
3. **Score.** The weighted average goes through a logistic curve. It is calculated twice: with candles only and with derivatives added. A factor with no recent reading is left out.
4. **Call.** Up or Down when the score is far enough from 50, otherwise Neutral. Confidence falls when factors are missing. Expected move comes from ATR over the horizon, and support and resistance from recent swing points.

A coin needs 200 candles, and until then shows `Not enough history`. One whose candles have an unexplained hole is skipped, and says so, until the hole is filled.

## Score and probability

A raw score is not a probability, so it shows as `Score 68`. A strategy and interval can earn a curve from isotonic regression, which maps scores to what actually happened and changes the display to `68%`. Earning one takes two things:

- **200 separate moments.** A moment is one horizon's stretch of time, so at 4 bars that is about 8 days at 15m, 33 days at 1h, 133 days at 4h and 2.2 years at 1d. Coins move together, so more coins in one moment do not count for more.
- **A hold-out.** A curve fitted on the older 70% of moments must predict the newest 30% better than simply knowing how often price rose. Scores with no relation to what happened do not pass, and stay scores.

The check runs hourly on verified live forecasts only. A curve applies to forecasts made after it, and is removed if it stops passing.

## Checking the calls

Every closed candle of a tracked series gets one record per strategy in use. Forecasts run up to `horizon` bars ahead, 4 by default. When the target candle closes, the call is judged against its close: `Hit` if the direction was right, `Miss` if it was wrong, and `Not judged` if it was Neutral or price did not move. The actual move is kept either way.

## Strategies

A saved strategy's rules never change, so its records always describe one set of rules, and they read as plain values. Its name can change. To try something else, press **New strategy** under the list. A new strategy starts from the one in view, or from any other or the default rules, and is saved under a name of its own. Backtest it, then put it to use. A draft survives leaving the page. A strategy not in use can be deleted, with its records and backtests.

Each entry in the lists of strategies and runs has a menu on hover with its actions. A double click, or the pencil beside the name on its own page, edits the name in place: Enter or leaving the field saves, and Escape keeps the old name. Strategies need a name of their own. A run with no name of its own goes by its strategy's.

**Performance** sets how a strategy has done live beside its latest backtest: independent calls, hit rate against chance and return after fees, overall and by interval, market, call and confidence. It also shows how many of the 200 moments each interval has towards a calibration curve. An Up or Down call is compared with how often price rose or fell overall.

When the forecast code changes what a strategy computes, an internal `EngineVersion` goes up. The next start copies the strategy in use under a new name, such as `Default (2)`, and puts the copy to use, so one strategy's records always come from one version of the code. The old strategy shows as `Retired`: it keeps its records and cannot be backtested, and a new strategy can start from its rules. Turning it back on, after a warning, puts the same strategy to use under the current code and deletes its signals, statistics, curves and backtests, since they came from the old code.

One strategy is in use at a time, and turning one on replaces the last. Turning the one in use off leaves none: nothing new is forecast or alerted until one is turned on, calls already made are still judged, and market data keeps collecting. The entry's menu and its panel's header both have this switch.

Strategies and their backtests are in `forecast.strategies` and `forecast.backtests`. No other feature reads them: a trader's settings are its own.

The config carries a `scorer` field, only `weighted` so far. Another way of scoring, including a model, can use the same strategies, records, verification and backtests.

## Backtests

A backtest reads only local data, one at a time, in the background, and can be stopped while it runs. **New backtest**, under the list of runs, takes the last 3, 6, 12 or 24 months or any range, so a strategy tuned on one stretch can be tested on another. Where local history is short, the form says so, and it can download history, with derivatives unless turned off. See [deep history](market-data.md#deep-history).

Each run in the list shows its hit rate against chance and its lift, for comparing them. A run with nothing to test says so. **Run again** opens the form with what a run tested, and a run that is not going can be deleted.

```bash
vergex forecast-backtest --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24
```

| Metric | Meaning |
| :----- | :------ |
| Samples | Non-overlapping tests: each series every `horizon` bars, so one move is not counted several times |
| Hit rate | Hits over directional calls that moved |
| By chance | What the same mix of Up and Down calls would hit with no skill. This is what a hit rate has to beat |
| Rose anyway | How often price simply rose, over the samples where it moved |
| Brier score, lift | Error of the scores, and the improvement over always predicting the base rate. Zero or below means no skill beyond it |
| Per signal after fees | Average return of acting on each call, less 0.10% round-trip fees |
| Within expected move | How often the actual move stayed inside the expected move |
| Derivatives | The share of samples that had derivatives readings. Binance's API keeps only about 30 days, so without deep history a long run tests mostly candle factors |
| Brier on candles alone | The same samples without derivatives, to show what they add |
| Factors | For each factor in each kind of market, how often its sign matched the move and its correlation with the move. A factor at about 50% and 0 carries nothing; one well below 50% reads better the other way round |
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

Watchlist coins alert over Telegram when a call's score crosses the threshold in **Forecast → Settings**: at 70, that means an Up call at 70 and above or a Down call at 30 and below. A neutral call does not alert, nor does a score a curve has put on the other side of the call. Each coin and interval alerts once, checked only when a candle closes, and can alert again after its score comes back inside. After downtime only the newest candle can alert, and only if it has just closed. Turn it off with Forecast signals in **Settings → Telegram**.
