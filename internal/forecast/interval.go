package forecast

import (
	"slices"

	"vergex/internal/market/feed"
)

// intervalMillis returns a Forecast interval's length, or 0 for any other interval.
func intervalMillis(interval string) int64 {
	if !slices.Contains(Intervals, interval) {
		return 0
	}
	return feed.IntervalMillis(interval)
}
