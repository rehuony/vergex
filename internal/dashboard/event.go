// Package dashboard turns what the system records into a stream of events for the
// Dashboard: model calls, orders, money and market reads, each with where it came
// from and where it went. It reads the database and the market streams' counters, so
// it never touches the trading loop and makes no exchange requests.
package dashboard

// Kinds, the groups the Dashboard filters by.
const (
	KindAgent  = "agent"
	KindOrders = "orders"
	KindFunds  = "funds"
	KindMarket = "market"
)

// Event types, which pick the wording of an event.
const (
	TypeAgentCall  = "agent_call"
	TypeAgentCost  = "agent_cost"
	TypeDecision   = "decision"
	TypeMarketRead = "market_read"
	TypeForecast   = "forecast"
	TypeOrder      = "order"
	TypePlanOrder  = "plan_order"
	TypeFee        = "fee"
	TypeMargin     = "margin"
	TypeRealized   = "realized"
)

// Blocks of the Dashboard's scene.
const (
	// BlockExchange is where market data comes from. Only the stream's flow reaches it.
	BlockExchange  = "exchange"
	BlockMarket    = "market"
	BlockTelegram  = "telegram"
	BlockTraders   = "traders"
	BlockFutures   = "futures"
	BlockForecast  = "forecast"
	BlockModels    = "models"
	BlockAccounts  = "accounts"
	BlockPositions = "positions"
	BlockAgentCost = "agent_cost"
	BlockFees      = "fees"
	// BlockOther is a call outside the scene, such as a model test or the assistant in the
	// web app.
	BlockOther = "other"
)

// Statuses of an event with no exchange status of its own.
const (
	StatusOK     = "ok"
	StatusFailed = "failed"
)

// Endpoint is one end of an event: a block, and the member of it when there is one.
type Endpoint struct {
	Block string `json:"block"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
}

// Event is one thing that happened.
type Event struct {
	Seq    int64    `json:"seq"`
	ID     string   `json:"id"`
	At     int64    `json:"at"` // Unix milliseconds.
	Kind   string   `json:"kind"`
	Type   string   `json:"type"`
	From   Endpoint `json:"from"`
	To     Endpoint `json:"to"`
	Symbol string   `json:"symbol,omitempty"`
	// Symbols are the coins a grouped event covers, up to maxEventSymbols of them.
	Symbols  []string `json:"symbols,omitempty"`
	Action   string   `json:"action,omitempty"`
	Quantity float64  `json:"quantity,omitempty"`
	Tokens   int      `json:"tokens,omitempty"`
	Items    int      `json:"items,omitempty"`  // Coins read, or actions decided.
	Amount   float64  `json:"amount,omitempty"` // In Asset. Fees and agent costs are positive.
	Asset    string   `json:"asset,omitempty"`
	Status   string   `json:"status,omitempty"`
	Detail   string   `json:"detail,omitempty"`
	Ref      string   `json:"ref,omitempty"` // The decision record or plan it opens.
	// TraderID is the trader whose page shows this, when one does.
	TraderID string `json:"trader_id,omitempty"`
}

// RouteStream is the line the market streams' flow runs along.
const RouteStream = BlockExchange + ">" + BlockMarket

// Route names the line an event runs along.
func (e Event) Route() string { return e.From.Block + ">" + e.To.Block }

// Total adds up a route's events over a window.
type Total struct {
	Count  int     `json:"count"`
	Tokens int64   `json:"tokens,omitempty"`
	Items  int64   `json:"items,omitempty"`
	Amount float64 `json:"amount,omitempty"`
}

// RouteTotal is a route's totals over the last hour and the last day.
type RouteTotal struct {
	H1  Total `json:"h1"`
	H24 Total `json:"h24"`
}

// StreamCount is what the market streams report: how much they have delivered since they
// started, and whether they are up.
type StreamCount struct {
	Messages  uint64
	Topics    int
	Connected bool
}

// StreamFlow is what the market streams delivered in the feed's latest round. It is not
// an event: it comes every few seconds and would push real events off the list.
type StreamFlow struct {
	At        int64  `json:"at"` // Unix milliseconds.
	Messages  uint64 `json:"messages"`
	Topics    int    `json:"topics"`
	Connected bool   `json:"connected"`
}

// Update is what a reader of the feed has not seen yet.
type Update struct {
	Seq    int64                 `json:"seq"`
	At     int64                 `json:"at"`
	Events []Event               `json:"events"`
	Totals map[string]RouteTotal `json:"totals"`
	// Stream is the latest round's flow. Nil when market data is unavailable.
	Stream *StreamFlow `json:"stream,omitempty"`
}
