package binance

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/trading"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/shopspring/decimal"
)

const (
	// fillBatch is the most trades one read returns.
	fillBatch = 1000
	// tradeWindow is the longest span Binance answers for a symbol's trades by time.
	tradeWindow = 7 * 24 * time.Hour
	// historyLimit is how far back Binance keeps trades and income: three months, less a
	// little so a request is never just past the edge.
	historyLimit = 89 * 24 * time.Hour
)

// FillsSince reads the account's trades since the cursor's time. Binance only lists
// trades per symbol, so it first works out which symbols to ask about. A symbol that
// cannot be read, or a list that could not be asked for, is reported in the error;
// the fills of the others still come back.
func (t *FuturesTrader) FillsSince(cursor trading.FillCursor) ([]trading.Fill, error) {
	since := cursor.Since
	if floor := time.Now().Add(-historyLimit); since.Before(floor) {
		logger.Warnf("Binance keeps three months of trades: anything on this account before %s can no longer be read", floor.UTC().Format(time.RFC3339))
		since = floor
	}

	symbols := make(map[string]bool)
	var failures []error
	// Income shows what traded, but misses some trades (VIP users, BNB discount,
	// zero-fee trades), so open positions count too, which also catches a fully closed
	// position through its P/L. A position only the ledger still has open closed while
	// nothing was reading, or income from that time has aged out.
	if found, err := t.GetCommissionSymbols(since); err != nil {
		failures = append(failures, fmt.Errorf("read commission symbols: %w", err))
	} else {
		for _, symbol := range found {
			symbols[symbol] = true
		}
	}
	for _, symbol := range t.getPositionSymbols() {
		symbols[symbol] = true
	}
	if found, err := t.GetPnLSymbols(since); err != nil {
		failures = append(failures, fmt.Errorf("read P/L symbols: %w", err))
	} else {
		for _, symbol := range found {
			symbols[symbol] = true
		}
	}
	for _, symbol := range cursor.OpenSymbols {
		symbols[symbol] = true
	}

	var fills []trading.Fill
	for symbol := range symbols {
		trades, err := t.tradesSince(symbol, since)
		if err != nil {
			failures = append(failures, fmt.Errorf("read trades of %s: %w", symbol, err))
			continue
		}
		for _, trade := range trades {
			fill, err := fillFromTrade(trade)
			if err != nil {
				// Reading it again would say the same.
				fill = trading.Fill{
					TradeID:      strconv.FormatInt(trade.ID, 10),
					OrderID:      strconv.FormatInt(trade.OrderID, 10),
					Symbol:       instrument.Normalize(trade.Symbol),
					NativeSymbol: trade.Symbol,
					Side:         strings.ToLower(string(trade.Side)),
					Time:         timeFromMillis(trade.Time),
					Unbookable:   fmt.Sprintf("%s %s at %s: %v", trade.Side, trade.Quantity, trade.Price, err),
				}
			}
			fills = append(fills, fill)
		}
	}
	return fills, errors.Join(failures...)
}

// tradesSince reads a symbol's trades from a time until now, a window at a time.
func (t *FuturesTrader) tradesSince(symbol string, since time.Time) ([]*futures.AccountTrade, error) {
	var all []*futures.AccountTrade
	now := time.Now()
	for start := since; start.Before(now); start = start.Add(tradeWindow) {
		end := start.Add(tradeWindow)
		if end.After(now) {
			end = now
		}
		trades, err := t.tradesBetween(symbol, start.UnixMilli(), end.UnixMilli())
		if err != nil {
			return nil, err
		}
		all = append(all, trades...)
	}
	return all, nil
}

// tradesBetween reads a symbol's trades in a span. Binance does not say which end of a
// span a full batch comes from, so a full batch is read again in halves. It goes by time
// and not fromId: go-binance sends that one as fromID, and nothing says Binance takes it.
func (t *FuturesTrader) tradesBetween(symbol string, start, end int64) ([]*futures.AccountTrade, error) {
	trades, err := t.client.NewListAccountTradeService().Symbol(symbol).StartTime(start).EndTime(end).Limit(fillBatch).Do(context.Background())
	if err != nil {
		return nil, err
	}
	if len(trades) < fillBatch {
		return trades, nil
	}
	if end <= start {
		return nil, fmt.Errorf("a full batch of trades at one moment (%d) cannot be split", start)
	}
	middle := start + (end-start)/2
	earlier, err := t.tradesBetween(symbol, start, middle)
	if err != nil {
		return nil, err
	}
	later, err := t.tradesBetween(symbol, middle+1, end)
	if err != nil {
		return nil, err
	}
	return append(earlier, later...), nil
}

func fillFromTrade(trade *futures.AccountTrade) (trading.Fill, error) {
	price, err := decimal.NewFromString(trade.Price)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("price: %w", err)
	}
	quantity, err := decimal.NewFromString(trade.Quantity)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("quantity: %w", err)
	}
	quote, err := decimal.NewFromString(trade.QuoteQuantity)
	if err != nil {
		quote = price.Mul(quantity)
	}
	commission, err := decimal.NewFromString(trade.Commission)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("commission: %w", err)
	}
	// Binance reports 0 for a trade that closes nothing.
	var realized decimal.NullDecimal
	if pnl, err := decimal.NewFromString(trade.RealizedPnl); err == nil && !pnl.IsZero() {
		realized = decimal.NewNullDecimal(pnl)
	}
	positionSide := strings.ToLower(string(trade.PositionSide))
	if positionSide != "long" && positionSide != "short" {
		positionSide = ""
	}
	return trading.Fill{
		TradeID:         strconv.FormatInt(trade.ID, 10),
		OrderID:         strconv.FormatInt(trade.OrderID, 10),
		Symbol:          instrument.Normalize(trade.Symbol),
		NativeSymbol:    trade.Symbol,
		Side:            strings.ToLower(string(trade.Side)),
		PositionSide:    positionSide,
		Price:           price,
		Quantity:        quantity,
		QuoteQuantity:   quote,
		Commission:      commission,
		CommissionAsset: trade.CommissionAsset,
		RealizedPnL:     realized,
		IsMaker:         trade.Maker,
		Time:            timeFromMillis(trade.Time),
	}, nil
}

// getPositionSymbols returns list of symbols that have active positions
// Used as fallback when COMMISSION detection fails
func (t *FuturesTrader) getPositionSymbols() []string {
	positions, err := t.GetPositions()
	if err != nil {
		return nil
	}

	var symbols []string
	for _, pos := range positions {
		if symbol, ok := pos["symbol"].(string); ok && symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}
