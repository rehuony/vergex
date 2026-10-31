// Package servicestate holds what each background service last reported, so the
// dashboard can show their health without reaching into them.
package servicestate

import (
	"sort"
	"sync"
	"time"
)

// State is how a background service is doing.
type State string

const (
	Running  State = "running"
	Off      State = "off"      // Not configured or not in use; nothing is wrong.
	Degraded State = "degraded" // Working from old data or retrying.
	Failed   State = "failed"
)

// Names of the services that report themselves.
const (
	Telegram       = "telegram"
	TelegramAlerts = "telegram_alerts"
	AIPricing      = "ai_pricing"
	MarketData     = "market_data"
	Forecast       = "forecast"
)

// Entry is one service's latest report.
type Entry struct {
	Name      string    `json:"name"`
	State     State     `json:"state"`
	Detail    string    `json:"detail,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	mu      sync.RWMutex
	entries = map[string]Entry{}
)

// Set records a service's state.
func Set(name string, state State, detail string) {
	mu.Lock()
	defer mu.Unlock()
	entries[name] = Entry{Name: name, State: state, Detail: detail, UpdatedAt: time.Now().UTC()}
}

// All returns every report, in a stable order.
func All() []Entry {
	mu.RLock()
	defer mu.RUnlock()
	all := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		all = append(all, entry)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all
}
