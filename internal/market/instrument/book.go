package instrument

import "time"

// BookLevel is one price level of an order book, with its numbers exactly as the venue
// wrote them.
type BookLevel struct {
	Price, Quantity string
}

// Book is a contract's order book at a moment and the rules for trading it, read from the
// venue rather than guessed from a symbol. Quantities are in the base asset.
type Book struct {
	BaseAsset, QuoteAsset string
	// ContractValue is how much base asset one contract is, where a venue counts in contracts.
	ContractValue       string
	Tick, Step          string
	MinQty, MinNotional string
	Bid, BidSize, Ask   string
	AskSize             string
	Bids, Asks          []BookLevel
	// SourceTime is when the venue says the book was taken, if it says.
	SourceTime time.Time
}

// ExactCandle is a candle with its numbers exactly as the venue wrote them. Time is in
// Unix seconds.
type ExactCandle struct {
	Time                           int64
	Open, High, Low, Close, Volume string
}

// Listing is a contract a venue offers, and how much it traded in a day.
type Listing struct {
	Symbol    string
	Base      string
	Volume24H float64
	Price     string
}

// Listings is what a venue offers and where that was read from.
type Listings struct {
	Markets  []Listing
	Sources  []string
	Warnings []string
}

// Reading is one derivatives figure and when the venue took it.
type Reading struct {
	Value  string
	Mark   string // The mark price, where a venue reports it with the figure.
	At     time.Time
	Source string // The endpoint it was read from.
}
