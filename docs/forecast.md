# Forecast

Forecast scores where a coin may go next from local market data, records each call and checks it later. It is an experiment to find out whether such signals have any value. It is not a proven strategy, nothing trades on it, and the numbers below say so.

Open **Forecast** in the top bar. Data comes from [market data](market-data.md), so a page or a backtest never adds exchange requests, apart from a price read when the live stream is down.

## Pages

| Page | Shows |
| :--- | :---- |
| Signals | Every tracked coin in a list that stays beside the page, then one coin's chart filling the screen with its call at the head, and what the call rests on, its factors and its record below. `/forecast` opens the last coin looked at, and `/forecast?symbol=BTCUSDT&interval=1h` a given one |
| Strategies | Saved strategies in a list that stays beside the page, then how one has done live and its rules |
| Backtests | Past runs in a list that stays beside the page, then one run's report, or the form to start another. Before the first run, the form is all there is |
| Settings | What is collected, saved with its button, and the alert threshold and intervals, saved as they change, beside how collection and the live stream are doing |

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
| Past calls | An arrow at each judged call the statistics count, pointing the call's way: green right, red wrong |
| Forecast band | The forecast over its horizon, from the last close to the candle that will judge it |
| EMA 20, 50; Bollinger 20, 2; VWAP 24; RSI 14 | The series behind the factors, worked out the same way as a forecast at each candle, and fetched only when one is on. RSI has a pane of its own |

The forecast band restates the reading and adds no information. It runs from the last close to the target candle, widening as the square root of time to the expected move on either side, with its middle drifting in the call's direction as far as the score says, and it takes the call's colour: green for Up, red for Down, grey for Neutral, when it drifts nowhere, as it also does where a calibrated score sits on the other side of 50 from the call. Nothing is stored for it. Only the direction of these calls has been tested, not the size or the path, so it is drawn see-through.

The reading shows this strategy's hit rate on the coin next to what the same mix of Up and Down calls would hit by chance, given how often price rose. A hit rate means nothing until it beats that: always calling Down in a falling market hits often, with no skill. Both count only forecasts whose windows do not overlap, the same way a backtest does. The record's header says how many of them were right, and adds the percentages from 30 on.

The Futures chart can overlay the same band and reading: see [Futures](futures.md).

The coin list orders strong calls first and keeps rows in place while values refresh, so nothing moves under the pointer: it reorders only on a new interval or sort. It shows each coin's price, 24-hour change and score, with the change blank while the live stream is down, and can be narrowed to the watchlist, to Up calls or to Down calls. Typing a coin that is not tracked and pressing Enter opens it and starts fetching it. In the search box, the up and down keys move through the list, Enter opens the row they are on, and Escape clears the search.

The star beside a coin puts it on the watchlist, shown in Chinese as `收藏`, or takes it off. A starred coin is collected at every interval and can alert, so it keeps a forecast whatever its volume does. The watchlist holds up to 100 coins.

The chart's head gives the price, the 24-hour change, the call and its score. The panel under the chart gives the strength, the expected move, when the call will be judged, support and resistance with their distance from the price, the market and its ADX, the hit rate with its sample size, what chance would hit, the funding rate, the open interest change, the long/short ratio against its own recent range, and when the forecast was made. It marks a series that has fallen behind as stale and keeps its last values. Factors lists each factor's reading, weight and effect on the score, and those with no reading.

## How a forecast is made

`forecast.Evaluate` is a pure function of a strategy and the data up to a candle: the last 200 candles, and the derivatives readings as of the candle's close less one hour, since a reading from the close itself only arrives after it. Live forecasts and backtests call it the same way, so a candle gets the same answer either way, and nothing after that candle can affect it. The exception is a window across a gap Binance confirmed it has no candles for: live forecasts accept it, and a backtest skips it.

1. **Factors.** Each reads from -1 (points down) to 1 (points up): trend, momentum, RSI, Bollinger position, recent returns, volume, candle shape, swing structure, stochastic, VWAP distance, divergence, funding rate, open interest and long/short ratio.
2. **Market.** ADX splits trending from ranging markets, and each has its own weights. A negative weight reads a factor the other way round: an overbought RSI supports a trend but argues against a range.
3. **Score.** The weighted average goes through a logistic curve. It is calculated twice: with candles only and with derivatives added. A factor with no recent reading is left out.
4. **Call.** Up or Down when the score is far enough from 50, otherwise Neutral. Strength is how far the score is from 50, less when factors are missing, and is no probability: it says nothing the score does not. Expected move is 0.55 of ATR14 scaled to the horizon, a band the actual move stays within about two times in three, and support and resistance come from recent swing points.

The default rules read every candle factor as reversal, with the same weights in trending and ranging markets, and leave swing structure and the derivatives factors off. On 20 large coins over 2024 to 2026, price that had just risen, sat high in its range or above its averages tended to come back a little over the next few bars at 15m, 1h and 4h, in both kinds of market, and the same factors read as continuation lost to chance; the derivatives factors added nothing measurable. Their slope of 0.5 keeps the score close to a probability, so most calls score between 44 and 56, and a direction is called from one point off 50. See [First results](#first-results).

A coin needs 200 candles, and until then shows `Not enough history`. One whose candles have an unexplained hole is skipped, and says so, until the hole is filled.

## Score and probability

A raw score is not a probability, so it shows as `Score 68`. The default rules keep it close to one, since their slope was fitted to what happened; a strategy with a steeper slope spreads its scores further than its hit rate warrants, which a backtest's reliability table shows. A strategy and interval can earn a curve from isotonic regression, which maps scores to what actually happened and changes the display to `68%`. Earning one takes two things:

- **200 separate moments.** A moment is one horizon's stretch of time, so at 4 bars that is about 8 days at 15m, 33 days at 1h, 133 days at 4h and 2.2 years at 1d. Coins move together, so more coins in one moment do not count for more.
- **A hold-out.** A curve fitted on the older 70% of moments must predict the newest 30% better than simply knowing how often price rose. Scores with no relation to what happened do not pass, and stay scores.

The check runs hourly on verified live forecasts only. A curve applies to forecasts made after it, and is removed if it stops passing.

## Checking the calls

Every closed candle of a tracked series gets one record per strategy in use. Forecasts run up to `horizon` bars ahead, 4 by default. When the target candle closes, the call is judged against its close: `Hit` if the direction was right, `Miss` if it was wrong, and `Not judged` if it was Neutral or price did not move. The actual move is kept either way.

## Strategies

A saved strategy's rules never change, so its records always describe one set of rules, and they read as plain values. Its name can change. To try something else, press **New strategy** under the list. A new one needs a weight in both trending and ranging markets, since a market with none would never get a call, and can look at most 96 bars ahead at 15m, 72 at 1h, 42 at 4h and 30 at 1d, with the fields saying about how long that is. Strategies saved before these limits keep working. The last strategy cannot be deleted, since nothing would be left to use. A new strategy starts from the one in view, or from any other or the default rules, and is saved under a name of its own. Backtest it, then put it to use. A draft survives leaving the page. A strategy not in use can be deleted, with its records and backtests.

Each entry in the lists of strategies and runs has a menu on hover with its actions. A double click, or the pencil beside the name on its own page, edits the name in place: Enter or leaving the field saves, and Escape keeps the old name. Strategies need a name of their own. A run with no name of its own goes by its strategy's.

**Performance** sets how a strategy has done live beside its latest backtest: independent calls, the moments they come from, hit rate against chance and return after fees, overall and by interval, market, call and strength. Figures from fewer than 30 moments are grey and take no colour: coins move together, so a hundred coins over one stretch of time say little more than one. It also shows, for each interval, how many of the 200 moments it has towards a calibration curve and where that stands: a score, a score that has the moments and did not pass the check, one that has the moments and is only checked while the strategy is in use, or a probability since the day its curve was built. An Up or Down call is compared with how often price rose or fell overall.

When the forecast code changes what a strategy computes, an internal `EngineVersion` goes up. The next start copies the strategy in use under a new name, such as `Default (2)`, and puts the copy to use, so one strategy's records always come from one version of the code. The old strategy shows as `Retired`: it keeps its records and cannot be backtested, and a new strategy can start from its rules. Turning it back on, after a warning, puts the same strategy to use under the current code and deletes its signals, statistics, curves and backtests, since they came from the old code.

One strategy is in use at a time, and turning one on replaces the last. Turning the one in use off leaves none: nothing new is forecast or alerted until one is turned on, calls already made are still judged, and market data keeps collecting. The entry's menu and its panel's header both have this switch.

Strategies and their backtests are in `forecast.strategies` and `forecast.backtests`. No other feature reads them: a trader's settings are its own.

The config carries a `scorer` field, only `weighted` so far. Another way of scoring, including a model, can use the same strategies, records, verification and backtests.

## Backtests

A backtest reads only local data, one at a time, in the background, and can be stopped while it runs. **New backtest**, under the list of runs, takes the last 3, 6, 12 or 24 months or any range, so a strategy tuned on one stretch can be tested on another. Where local history is short, the form says so and offers to download it, with derivatives unless turned off. With nothing short it offers neither, and `vergex market-backfill --derivatives` fills derivatives in for history already held. See [deep history](market-data.md#deep-history). The list holds the newest 100 runs, and a link to any other says it was not found.

A run's report gives the range it really tested, and the list marks a run where local history made that much less than what it asked for: a year asked for and three days tested reads as neither. Each run in the list shows its hit rate against chance and its lift, for comparing them. A run with nothing to test says so. **Run again** opens the form with what a run tested, and a run that is not going can be deleted.

```bash
vergex forecast-backtest --symbols BTCUSDT,ETHUSDT --intervals 1h,4h,1d --months 24
```

| Metric | Meaning |
| :----- | :------ |
| Samples | Non-overlapping tests: each series every `horizon` bars, at candles a whole number of horizons from the epoch, so one move is not counted several times and every coin and every run tests the same candles |
| Moments | The separate stretches of time, one horizon long, that the calls come from. Coins move together, so this is how much evidence there really is, and figures from fewer than 30 are grey. A report from before they were counted shows `—` |
| Hit rate | Hits over directional calls that moved |
| By chance | What the same mix of Up and Down calls would hit with no skill. This is what a hit rate has to beat |
| Rose anyway | How often price simply rose, over the samples where it moved |
| Brier score, lift | Error of the scores, and the improvement over always predicting the base rate. A check of how honest the scores are, not of skill: a forecaster right 52% of the time can lift at most 0.2%, so about zero means the scores say what they should, and well below zero means they claim more than they deliver |
| Per signal after fees | Average return of acting on each call, less 0.10% round-trip fees |
| Within expected move | How often the actual move stayed inside the expected move. About two in three is what the band is sized for; a run from before `EngineVersion` 3 shows about nine in ten, since the band was then the whole ATR |
| Derivatives | The share of samples that had derivatives readings. Binance's API keeps only about 30 days, so without deep history a long run tests mostly candle factors |
| Brier on candles alone | The same samples without derivatives, to show what they add |
| Factors | For each factor in each kind of market, how often its sign matched the move and its correlation with the move. A factor at about 50% and 0 carries nothing; one well below 50% reads better the other way round |
| Reliability | For each range of scores, what was said against how often price rose |

### First results

The default rules were set from a measurement, not a guess: 20 USDT perpetuals (the default watchlist with LTC, BCH, DOT, UNI, AAVE, NEAR, APT, ARB, OP and TRX) over the 24 months to 2026-11-03 at 15m, 1h, 4h and 1d, candles only, under `EngineVersion` 3, through the same code as `vergex forecast-backtest`:

| | Samples | Moments | Hit rate | By chance | Rose anyway | Lift | Per signal after fees | Within expected move |
| :- | --: | --: | --: | --: | --: | --: | --: | --: |
| Overall | 460,553 | 23,042 | 51.8% | 50.0% | 49.7% | -0.1% | -0.101% | 67% |
| 15m | 350,093 | 17,519 | 51.9% | 50.0% | 49.7% | -0.1% | -0.101% | 67% |
| 1h | 86,860 | 4,343 | 51.6% | 50.0% | 49.9% | -0.2% | -0.102% | 68% |
| 4h | 20,960 | 1,048 | 52.7% | 49.9% | 49.5% | +0.2% | -0.053% | 66% |
| 1d | 2,640 | 132 | 49.6% | 49.3% | 46.2% | -1.3% | -0.460% | 69% |
| Ranging | 253,372 | 21,811 | 51.6% | 50.0% | 49.5% | -0.2% | -0.098% | 66% |
| Trending | 207,181 | 22,297 | 52.2% | 50.0% | 50.0% | -0.1% | -0.104% | 68% |

The rules were chosen on the oldest 70% of this stretch and checked on the newest 30%, where they hit 1.7, 2.3 and 3.0 points over chance at 1h, 4h and 15m, on 17 of 18 coins at 1h and 10 of 10 at 15m, and in 7 of 9 quarters at 1h and 9 of 9 at 4h and 15m. The reliability table sits close to 50 because the scores do: scores of 46 rose 48% of the time and scores of 54 rose 51%, which is what the slope was set for. That is the whole of the edge there is in these factors, two to three points of hit rate, which at 15m and 1h does not pay the 0.10% round trip and at 4h nearly does. 1d has too few moments to say anything either way, so its calls are untested. The rules that came before read the same factors as continuation and hit 49.4% against 50.0% at 1h, and 47.9% where ADX said trending. Treat the signals as an experiment: the edge was found on two years of twenty large coins, and nothing says it holds on other coins or later.

## Alerts

Watchlist coins alert over Telegram when a call's score crosses the threshold in **Forecast → Settings**: at 56, the default, that means an Up call at 56 and above or a Down call at 44 and below, which under the default rules is about the strongest fifth of calls, the ones that hit five to six points over chance out of sample. An install that saved its settings before keeps them. A neutral call does not alert, nor does a score a curve has put on the other side of the call. Each coin and interval alerts once, checked only when a candle closes, and can alert again after its score comes back inside. After downtime only the newest candle can alert, and only if it has just closed. Only the intervals picked beside the threshold can alert, by default 1h and 4h: 15m calls would fill the chat, and 1d calls have too short a record to stand on. One round of candles is one message, in the language set with `/lang`: a table under the strategy's name with each coin's interval, call, about how long it looks ahead and its score. It arrives silently, as Forecast trades nothing. Turn it off with Forecast signals in **Settings → Telegram**.
