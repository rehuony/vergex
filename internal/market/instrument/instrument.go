// Package instrument names what can be traded. It has no other dependencies, because
// both the market data and the trading ledger refer to instruments.
package instrument

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Market kinds.
const (
	KindPerpetual = "perpetual"
	KindSpot      = "spot"
)

// Instrument is one venue, market kind and symbol. A venue's own name for it is worked
// out from the symbol where it is needed, so it is not kept here.
type Instrument struct {
	ID        int64 `gorm:"primaryKey;autoIncrement"`
	Venue     string
	Kind      string
	Symbol    string
	CreatedAt time.Time
}

func (Instrument) TableName() string { return "market.instruments" }

// Normalize normalizes symbol, ensuring it's a USDT trading pair
func Normalize(symbol string) string {
	symbol = strings.ToUpper(symbol)

	// Remove exchange-specific separators (Gate uses BTC_USDT, OKX uses BTC-USDT-SWAP)
	symbol = strings.ReplaceAll(symbol, "_", "")
	symbol = strings.ReplaceAll(symbol, "-SWAP", "")
	symbol = strings.ReplaceAll(symbol, "-", "")

	// For regular crypto assets
	if strings.HasSuffix(symbol, "USDT") {
		return symbol
	}
	return symbol + "USDT"
}

// Ensure returns an instrument's ID, creating it the first time it is seen.
func Ensure(db *gorm.DB, venue, kind, symbol string) (int64, error) {
	var id int64
	find := func() error {
		return db.Model(&Instrument{}).Where("venue = ? AND kind = ? AND symbol = ?", venue, kind, symbol).
			Limit(1).Pluck("id", &id).Error
	}
	if err := find(); err != nil {
		return 0, fmt.Errorf("find instrument %s: %w", symbol, err)
	}
	if id != 0 {
		return id, nil
	}
	row := Instrument{Venue: venue, Kind: kind, Symbol: symbol}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return 0, fmt.Errorf("create instrument %s: %w", symbol, err)
	}
	if row.ID != 0 {
		return row.ID, nil
	}
	if err := find(); err != nil {
		return 0, fmt.Errorf("find instrument %s: %w", symbol, err)
	}
	return id, nil
}
