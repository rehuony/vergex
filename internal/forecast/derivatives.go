package forecast

import (
	"math"
	"sort"
	"time"
)

// Reading is one timed derivatives value.
type Reading struct {
	Time  int64
	Value float64
}

// Readings holds a coin's derivatives history, oldest first.
type Readings struct {
	Funding   []Reading
	OpenValue []Reading
	LongShort []Reading
}

const (
	fundingMaxAge      = 12 * time.Hour
	longShortWindow    = 7 * 24 * time.Hour
	longShortMinPoints = 12
	// oiLookbackBars is the span of the open interest change, in bars.
	oiLookbackBars = 12
)

// latestAt returns the index of the newest reading at or before t, or -1.
func latestAt(readings []Reading, t int64) int {
	return sort.Search(len(readings), func(i int) bool { return readings[i].Time > t }) - 1
}

// DerivativesAt says what the readings showed at time t, using nothing later. It
// serves live forecasts and backtests alike, so both see the same thing.
func DerivativesAt(readings Readings, t, step int64) Derivatives {
	var out Derivatives
	if i := latestAt(readings.Funding, t); i >= 0 && t-readings.Funding[i].Time <= fundingMaxAge.Milliseconds() {
		value := readings.Funding[i].Value
		out.Funding = &value
	}

	maxAge := max(2*step, time.Hour.Milliseconds()*2)
	if i := latestAt(readings.OpenValue, t); i >= 0 && t-readings.OpenValue[i].Time <= maxAge {
		earlier := latestAt(readings.OpenValue, t-oiLookbackBars*step)
		if earlier >= 0 && t-oiLookbackBars*step-readings.OpenValue[earlier].Time <= maxAge && readings.OpenValue[earlier].Value > 0 {
			change := (readings.OpenValue[i].Value/readings.OpenValue[earlier].Value - 1) * 100
			out.OIChangePct = &change
		}
	}

	if i := latestAt(readings.LongShort, t); i >= 0 && t-readings.LongShort[i].Time <= maxAge {
		from := sort.Search(len(readings.LongShort), func(j int) bool {
			return readings.LongShort[j].Time >= t-longShortWindow.Milliseconds()
		})
		window := readings.LongShort[from : i+1]
		if len(window) >= longShortMinPoints {
			var sum, squares float64
			for _, reading := range window {
				sum += reading.Value
			}
			mean := sum / float64(len(window))
			for _, reading := range window {
				squares += (reading.Value - mean) * (reading.Value - mean)
			}
			if deviation := math.Sqrt(squares / float64(len(window))); deviation > 0 {
				z := (readings.LongShort[i].Value - mean) / deviation
				out.LongShortZ = &z
			}
		}
	}
	return out
}
