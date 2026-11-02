package binance

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Binance counts request weight per IP, and live trading on this host shares
// the allowance. Background market data therefore takes a small slice of it and
// steps aside as soon as the exchange reports heavy use.
const (
	// DefaultWeightBudget is the weight per minute for background requests,
	// 12.5% of the 2400 Binance allows.
	DefaultWeightBudget = 300
	// DefaultDataBudget is the requests per minute for /futures/data/*, which
	// Binance limits separately to 1000 per 5 minutes.
	DefaultDataBudget = 150
	// DefaultFundingBudget is the requests per minute for /fapi/v1/fundingRate,
	// which shares 500 per 5 minutes with fundingInfo.
	DefaultFundingBudget = 60
	// DefaultTradingBudget is the weight per minute for reads a running trader is
	// waiting on, kept apart so a long catch-up cannot queue them.
	DefaultTradingBudget = 600

	// usedWeightPause is the X-MBX-USED-WEIGHT-1M reading above which background
	// requests wait for the next minute. It includes live trading's traffic, and
	// trading's own reads do not wait for it: well under the real limit, they matter
	// more than anything queued behind them.
	usedWeightPause = 1200

	minRateBackoff = time.Minute
	maxRateBackoff = 30 * time.Minute
	minBanPause    = time.Hour
)

// Budget is a separately counted class of requests.
type Budget int

const (
	BudgetWeight Budget = iota
	BudgetData
	BudgetFunding
	BudgetTrading
)

// LimiterStatus is what the limiter last knew, for status displays.
type LimiterStatus struct {
	PausedUntil time.Time `json:"paused_until,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Banned      bool      `json:"banned"`
	UsedWeight  int       `json:"used_weight"`
}

// Limiter paces this process's background requests. It is safe for concurrent use.
type Limiter struct {
	mu          sync.Mutex
	now         func() time.Time
	budgets     map[Budget]int
	used        map[Budget]int
	minute      int64
	pausedUntil time.Time // Everything waits: the exchange said to stop.
	busyUntil   time.Time // Background waits: the allowance is running low.
	reason      string
	busyReason  string
	backoff     time.Duration
	banned      bool
	usedWeight  int
}

// NewLimiter creates a limiter with the default budgets.
func NewLimiter() *Limiter {
	return &Limiter{
		now: time.Now,
		budgets: map[Budget]int{
			BudgetWeight:  DefaultWeightBudget,
			BudgetData:    DefaultDataBudget,
			BudgetFunding: DefaultFundingBudget,
			BudgetTrading: DefaultTradingBudget,
		},
		used: map[Budget]int{},
	}
}

// Acquire waits until a request costing this much may go out.
func (l *Limiter) Acquire(ctx context.Context, budget Budget, cost int) error {
	for {
		wait := l.reserve(budget, cost)
		if wait <= 0 {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *Limiter) reserve(budget Budget, cost int) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Before(l.pausedUntil) {
		return l.pausedUntil.Sub(now)
	}
	if budget != BudgetTrading && now.Before(l.busyUntil) {
		return l.busyUntil.Sub(now)
	}
	if minute := now.Unix() / 60; minute != l.minute {
		l.minute = minute
		l.used = map[Budget]int{}
	}
	if l.used[budget]+cost > l.budgets[budget] {
		return time.Unix((now.Unix()/60+1)*60, 0).Sub(now)
	}
	l.used[budget] += cost
	return 0
}

// Observe learns from a response: the weight header and any rate limit status.
func (l *Limiter) Observe(status int, header http.Header) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if used, err := strconv.Atoi(header.Get("X-MBX-USED-WEIGHT-1M")); err == nil {
		l.usedWeight = used
		if until := time.Unix((now.Unix()/60+1)*60, 0); used > usedWeightPause && until.After(l.busyUntil) {
			l.busyUntil = until
			l.busyReason = fmt.Sprintf("used weight %d", used)
		}
	}
	retryAfter := time.Duration(0)
	if seconds, err := strconv.Atoi(header.Get("Retry-After")); err == nil && seconds > 0 {
		retryAfter = time.Duration(seconds) * time.Second
	}
	switch {
	case status == http.StatusTeapot:
		wait := retryAfter
		if wait < minBanPause {
			wait = minBanPause
		}
		l.banned = true
		l.pause(now.Add(wait), "IP banned by Binance (418)")
	case status == http.StatusTooManyRequests || status == http.StatusForbidden:
		wait := retryAfter
		if wait <= 0 {
			l.backoff *= 2
			if l.backoff < minRateBackoff {
				l.backoff = minRateBackoff
			}
			if l.backoff > maxRateBackoff {
				l.backoff = maxRateBackoff
			}
			wait = l.backoff
		}
		l.pause(now.Add(wait), fmt.Sprintf("rate limited by Binance (%d)", status))
	case status >= 200 && status < 300:
		l.backoff = 0
		l.banned = false
	}
}

func (l *Limiter) pause(until time.Time, reason string) {
	if until.After(l.pausedUntil) {
		l.pausedUntil = until
		l.reason = reason
	}
}

// Status reports the current pause, if any, and the last reading.
func (l *Limiter) Status() LimiterStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	status := LimiterStatus{Banned: l.banned, UsedWeight: l.usedWeight}
	switch now := l.now(); {
	case now.Before(l.pausedUntil):
		status.PausedUntil, status.Reason = l.pausedUntil, l.reason
	case now.Before(l.busyUntil):
		status.PausedUntil, status.Reason = l.busyUntil, l.busyReason
	}
	return status
}
