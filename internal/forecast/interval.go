package forecast

var millis = map[string]int64{
	"15m": 15 * 60 * 1000,
	"1h":  60 * 60 * 1000,
	"4h":  4 * 60 * 60 * 1000,
	"1d":  24 * 60 * 60 * 1000,
}

func intervalMillis(interval string) int64 { return millis[interval] }
