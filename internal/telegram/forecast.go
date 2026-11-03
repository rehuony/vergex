package telegram

import (
	"context"
	"math"
	"strconv"
	"time"

	"vergex/internal/forecast"
)

// NotifyForecast tells the bound chat of the calls that crossed the threshold, silently:
// Forecast is experimental and trades nothing, so it is worth a look, not a sound.
func NotifyForecast(ctx context.Context, configs ConfigStore, alert forecast.Alert) error {
	return Notify(ctx, configs, forecastMessage(alert, configs.GetLanguage()))
}

func forecastMessage(alert forecast.Alert, lang string) *message {
	m := newMessage(lang)
	m.silent = true
	m.title(emojiForecast, counted(lang, "Forecast signals", "预测信号", len(alert.Signals)))
	// The interval goes with the symbol, as a column of its own would leave the score no room.
	t := table{caption: alert.Strategy, columns: []column{
		{name: pick(lang, "Symbol", "品种")}, {name: pick(lang, "Call", "方向")},
		{name: pick(lang, "Over", "时长")}, {name: pick(lang, "Score", "评分"), right: true},
	}}
	for _, signal := range alert.Signals {
		call := pick(lang, "up", "上涨")
		if signal.Direction == forecast.DirectionDown {
			call = pick(lang, "down", "下跌")
		}
		// A calibrated call is a chance, and reads as one; anything else is a score.
		value := strconv.Itoa(forecast.Score(signal.DisplayProb))
		if signal.Calibrated {
			value += "%"
		}
		t.rows = append(t.rows, []string{signal.Symbol + " " + signal.Interval, call, roughSpan(signal.Span()), value})
	}
	return m.table(t)
}

// roughSpan is a stretch of time in the largest unit that reads well, since nobody works out
// four 15m bars in their head.
func roughSpan(span time.Duration) string {
	hours := span.Hours()
	switch {
	case hours >= 48:
		return "~" + strconv.Itoa(int(math.Round(hours/24))) + " d"
	case hours >= 1:
		return "~" + strconv.Itoa(int(math.Round(hours))) + " h"
	}
	return "~" + strconv.Itoa(int(math.Round(span.Minutes()))) + " min"
}
