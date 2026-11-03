package assistant

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// The commands read the local API without the model, so they answer at once and cost
// nothing. What they read is here; how it reads is the surface's.

// Position is the part of a venue's position the commands show. Venues name the fields
// differently, and give numbers as strings or numbers, so each is looked for under its names.
type Position struct {
	Symbol, Side          string
	Quantity, Entry, Gain float64
	HasGain               bool
}

func readNumber(row map[string]any, names ...string) (float64, bool) {
	for _, name := range names {
		switch value := row[name].(type) {
		case float64:
			return value, true
		case json.Number:
			if number, err := value.Float64(); err == nil {
				return number, true
			}
		case string:
			if number, err := strconv.ParseFloat(value, 64); err == nil {
				return number, true
			}
		}
	}
	return 0, false
}

func readPosition(row map[string]any) Position {
	p := Position{Symbol: fmt.Sprint(row["symbol"]), Side: strings.ToLower(fmt.Sprint(row["side"]))}
	p.Quantity, _ = readNumber(row, "quantity", "positionAmt", "positionAmount", "size")
	p.Entry, _ = readNumber(row, "entry_price", "entryPrice")
	p.Gain, p.HasGain = readNumber(row, "unrealized_pnl", "unRealizedProfit", "unrealizedPnl")
	if p.Quantity < 0 {
		p.Quantity = -p.Quantity
	}
	return p
}

// readPositions reads one account's open positions.
func (a *Agent) readPositions(accountID string) ([]Position, error) {
	var rows []map[string]any
	if err := a.api.do("GET", "/api/accounts/"+url.PathEscape(accountID)+"/positions", nil, &rows); err != nil {
		return nil, err
	}
	out := make([]Position, len(rows))
	for i, row := range rows {
		out[i] = readPosition(row)
	}
	return out, nil
}

// Holding is one account as the commands show it: what it is worth and what it holds, each
// where it could be read.
type Holding struct {
	Name          string
	Testnet       bool
	Equity        float64
	EquityRead    bool
	Positions     []Position
	PositionsRead bool
}

// Holdings reads every account, whoever opened what it holds. It fails only when the accounts
// themselves cannot be read.
func (a *Agent) Holdings() ([]Holding, error) {
	accounts, err := a.accounts()
	if err != nil {
		return nil, err
	}
	states := a.accountStates()
	out := make([]Holding, len(accounts))
	for i, e := range accounts {
		h := Holding{Name: e.Name, Testnet: e.Testnet}
		if state, ok := states[e.ID]; ok && state.Status == "ok" {
			h.Equity, h.EquityRead = state.Equity, true
		}
		if positions, err := a.readPositions(e.ID); err == nil {
			h.Positions, h.PositionsRead = positions, true
		}
		out[i] = h
	}
	return out, nil
}

// TraderState is a trader as the commands show it.
type TraderState struct {
	Name, Kind string
	Running    bool
}

// TraderStates reads the traders and whether each runs.
func (a *Agent) TraderStates() ([]TraderState, error) {
	traders, err := a.traders()
	if err != nil {
		return nil, err
	}
	out := make([]TraderState, len(traders))
	for i, t := range traders {
		out[i] = TraderState{Name: t.Name, Kind: t.Kind, Running: t.Running}
	}
	return out, nil
}
