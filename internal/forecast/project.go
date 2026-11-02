package forecast

import "math"

// Project draws a forecast as candles over its horizon. It only restates the reading
// in another form, with no randomness and no new information: the score and expected
// move set how far the path goes, and each candle spans one typical bar's range. Only
// the direction of these calls has been tested against what happened, not the size or
// the path, so it is not stored and nothing is judged on it.
//
// Its first candle comes right after the candle the forecast was made from.
func Project(signal Signal, step int64) []CandleRow {
	if signal.Horizon <= 0 || signal.Close <= 0 || step <= 0 {
		return nil
	}
	expectedMove := signal.ExpectedMovePct / 100 * signal.Close
	typicalBar := expectedMove / math.Sqrt(float64(signal.Horizon))
	drift := (signal.DisplayProb - 0.5) / 0.5 * expectedMove / float64(signal.Horizon)
	// A curve can put the score on the other side of 50 from the call. The call is what
	// gets judged, so the path goes nowhere rather than against it.
	if signal.Direction == DirectionNeutral || (signal.Direction == DirectionUp) != (drift > 0) {
		drift = 0
	}
	wick := math.Max(0, typicalBar-math.Abs(drift)) / 2

	rows := make([]CandleRow, signal.Horizon)
	open := signal.Close
	for i := range rows {
		closePrice := open + drift
		rows[i] = CandleRow{
			Time: (signal.OpenTime + int64(i+1)*step) / 1000,
			Open: open, Close: closePrice,
			High: math.Max(open, closePrice) + wick,
			Low:  math.Max(0, math.Min(open, closePrice)-wick),
		}
		open = closePrice
	}
	return rows
}
