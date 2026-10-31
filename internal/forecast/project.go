package forecast

import "math"

// Project draws a forecast as candles over its horizon. It only restates the reading
// in another form, with no randomness and no new information: the score and expected
// move set how far the path goes, and each candle spans one typical bar's range. Only
// the direction of these calls has been tested against what happened, not the size or
// the path, so it is not stored and nothing is judged on it.
//
// prob is the score the reading shows, and lastOpen and step place the first candle
// right after the candle the forecast was made from.
func Project(result Result, prob float64, lastOpen, step int64) []CandleRow {
	if result.Horizon <= 0 || result.Close <= 0 || step <= 0 {
		return nil
	}
	expectedMove := result.ExpectedMovePct / 100 * result.Close
	typicalBar := expectedMove / math.Sqrt(float64(result.Horizon))
	drift := 0.0
	if result.Direction != DirectionNeutral {
		drift = (prob - 0.5) / 0.5 * expectedMove / float64(result.Horizon)
	}
	wick := math.Max(0, typicalBar-math.Abs(drift)) / 2

	rows := make([]CandleRow, result.Horizon)
	open := result.Close
	for i := range rows {
		closePrice := open + drift
		rows[i] = CandleRow{
			Time: (lastOpen + int64(i+1)*step) / 1000,
			Open: open, Close: closePrice,
			High: math.Max(open, closePrice) + wick,
			Low:  math.Max(0, math.Min(open, closePrice)-wick),
		}
		open = closePrice
	}
	return rows
}
