package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"vergex/internal/futures"
	"vergex/internal/futures/agent"
	"vergex/internal/futures/plan"
	"vergex/internal/market/feed"

	"github.com/gin-gonic/gin"
)

// chartPollEvery is how often a chart with no live data polls the exchange, and
// how often one on a quiet market hears that the feed is still alive.
const chartPollEvery = 3 * time.Second

// liveCharts is the live data a chart can follow.
type liveCharts interface {
	follow(venue, symbol, interval string) chartCandles
	healthy(venue string) bool
}

// chartCandles is one chart's subscription.
type chartCandles interface {
	Ready() <-chan struct{}
	Drain() []feed.LiveCandle
	Close()
}

type serviceCharts struct{ service *feed.Service }

func (s serviceCharts) follow(venue, symbol, interval string) chartCandles {
	if s.service == nil {
		return nil
	}
	if follower := s.service.FollowCandles(venue, symbol, interval); follower != nil {
		return follower
	}
	return nil
}

func (s serviceCharts) healthy(venue string) bool {
	return s.service != nil && s.service.StreamHealthy(venue)
}

func (s *Server) serveFuturesFeed(c *gin.Context, source futures.ChartSource, q futures.CandleQuery) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Accel-Buffering", "no")
	write := func(event agent.Event) error {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(c.Writer, "data: %s\n\n", raw); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	runChartFeed(c.Request.Context(), write, source, serviceCharts{s.marketData}, q, chartPollEvery, c.Query("live") == "1")
}

// runChartFeed sends a chart its candles until ctx ends or a write fails: what the
// exchange has now, then what the live stream delivers. The chart picks up any hole
// from the candles' times, so a stream that drops needs nothing more than polling
// until it is back. liveOnly is for a chart with candles of its own: it hears the
// stream and nothing else, so it never spends a request.
func runChartFeed(ctx context.Context, write func(agent.Event) error, source futures.ChartSource, live liveCharts, q futures.CandleQuery, pollEvery time.Duration, liveOnly bool) {
	send := func(rows []futures.Candle) error {
		return write(agent.Event{Type: "candles", Data: gin.H{"candles": rows, "venue": q.Venue, "symbol": q.Symbol, "interval": q.Interval, "mode": "public", "received": time.Now().UTC()}})
	}
	var latest []futures.Candle
	poll := func() error {
		rows, err := source.Candles(ctx, q)
		if err != nil {
			return write(agent.Event{Type: "feed_error", Data: gin.H{"message": err.Error()}})
		}
		if len(rows) > 0 {
			latest = rows[len(rows)-1:]
		}
		return send(rows)
	}

	// Following starts first, so nothing falls between the poll and the stream.
	var ready <-chan struct{}
	candles := live.follow(q.Venue, q.Symbol, q.Interval)
	if candles != nil {
		defer candles.Close()
		ready = candles.Ready()
	}
	if !liveOnly && poll() != nil {
		return
	}

	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ready:
			rows := chartRows(candles.Drain())
			if len(rows) == 0 {
				continue
			}
			latest = rows[len(rows)-1:]
			if send(rows) != nil {
				return
			}
		case <-tick.C:
			var err error
			if candles != nil && live.healthy(q.Venue) {
				if latest != nil {
					err = send(latest)
				}
			} else if !liveOnly {
				err = poll()
			}
			if err != nil {
				return
			}
		}
	}
}

func chartRows(candles []feed.LiveCandle) []futures.Candle {
	rows := make([]futures.Candle, len(candles))
	for i, candle := range candles {
		rows[i] = futures.Candle{
			Time: candle.OpenTime / 1000, Open: chartDecimal(candle.Open), High: chartDecimal(candle.High),
			Low: chartDecimal(candle.Low), Close: chartDecimal(candle.Close), Volume: chartDecimal(candle.Volume),
		}
	}
	return rows
}

func chartDecimal(value float64) plan.Decimal {
	return plan.Decimal(strconv.FormatFloat(value, 'f', -1, 64))
}
