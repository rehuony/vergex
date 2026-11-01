package okx

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"vergex/internal/logger"
	"vergex/internal/trading"

	"github.com/shopspring/decimal"
)

const (
	// fillBatch is the most OKX returns in one read.
	fillBatch = 100
	// fillPagePause keeps paging within OKX's limit of 10 reads every 2 seconds.
	fillPagePause = 250 * time.Millisecond
	// historyLimit is how far back OKX keeps fills: three months, less a little so a
	// request is never just past the edge.
	historyLimit = 89 * 24 * time.Hour
)

// FillsSince reads the account's trades since the cursor's time, across all symbols,
// paging back by bill ID until there are no more. A read that fails reports nothing: a
// trade left behind while newer ones move the read on would be lost. A trade that could
// never be read comes back marked Unbookable, so it holds nothing up.
func (t *OKXTrader) FillsSince(cursor trading.FillCursor) ([]trading.Fill, error) {
	since := cursor.Since
	if floor := time.Now().Add(-historyLimit); since.Before(floor) {
		logger.Warnf("OKX keeps three months of fills: anything on this account before %s can no longer be read", floor.UTC().Format(time.RFC3339))
		since = floor
	}

	var fills []trading.Fill
	after := ""
	for {
		path := fmt.Sprintf("/api/v5/trade/fills-history?instType=SWAP&limit=%d&begin=%d", fillBatch, since.UnixMilli())
		if after != "" {
			path += "&after=" + after
		}
		data, err := t.doRequest("GET", path, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get fills history: %w", err)
		}
		var rows []okxFill
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("failed to parse fills: %w", err)
		}
		for _, row := range rows {
			fill, err := t.fillFromRow(row)
			if err != nil {
				return nil, fmt.Errorf("read trade %s: %w", row.TradeID, err)
			}
			fills = append(fills, fill)
		}
		if len(rows) < fillBatch {
			return fills, nil
		}
		next := rows[len(rows)-1].BillID
		if next == "" || next == after {
			return nil, fmt.Errorf("cannot page past bill %q", next)
		}
		after = next
		time.Sleep(fillPagePause)
	}
}

// fillFromRow reads a row, whose sizes are in contracts. Without the contract's value
// there is no telling how much traded, so that is either a failed read, which the next
// one can settle, or a contract OKX no longer describes, which no read will.
func (t *OKXTrader) fillFromRow(row okxFill) (trading.Fill, error) {
	symbol := t.convertSymbolBack(row.InstID)
	inst, err := t.getInstrument(symbol)
	if err != nil && !errors.Is(err, errNoInstrument) {
		return trading.Fill{}, fmt.Errorf("read the contract of %s: %w", row.InstID, err)
	}
	if err != nil || inst.CtVal <= 0 {
		return row.unbookable(symbol, "OKX no longer describes this contract, so how much traded is unknown"), nil
	}
	fill, err := row.fill(symbol, decimal.NewFromFloat(inst.CtVal))
	if err != nil {
		return row.unbookable(symbol, err.Error()), nil
	}
	return fill, nil
}

func (row okxFill) unbookable(symbol, why string) trading.Fill {
	ts, _ := strconv.ParseInt(row.Ts, 10, 64)
	return trading.Fill{
		TradeID:      row.TradeID,
		OrderID:      row.OrdID,
		Symbol:       symbol,
		NativeSymbol: row.InstID,
		Side:         strings.ToLower(row.Side),
		Time:         time.UnixMilli(ts).UTC(),
		Unbookable:   fmt.Sprintf("%s %s contracts at %s: %s", row.Side, row.FillSz, row.FillPx, why),
	}
}

// okxFill is one row of OKX's fills history.
type okxFill struct {
	InstID   string `json:"instId"`   // e.g., "BTC-USDT-SWAP"
	TradeID  string `json:"tradeId"`  // Trade ID
	BillID   string `json:"billId"`   // Bill ID, what paging goes by
	OrdID    string `json:"ordId"`    // Order ID
	Side     string `json:"side"`     // buy or sell
	PosSide  string `json:"posSide"`  // long, short, or net
	FillPx   string `json:"fillPx"`   // Fill price
	FillSz   string `json:"fillSz"`   // Fill size (contracts)
	FillPnl  string `json:"fillPnl"`  // Realized P/L of a closing fill
	Fee      string `json:"fee"`      // Fee (negative for cost)
	FeeCcy   string `json:"feeCcy"`   // Fee currency
	Ts       string `json:"ts"`       // Trade timestamp (ms)
	ExecType string `json:"execType"` // T: taker, M: maker
}

func (row okxFill) fill(symbol string, contract decimal.Decimal) (trading.Fill, error) {
	price, err := decimal.NewFromString(row.FillPx)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("price: %w", err)
	}
	size, err := decimal.NewFromString(row.FillSz)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("size: %w", err)
	}
	fee, err := decimal.NewFromString(row.Fee)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("fee: %w", err)
	}
	ts, err := strconv.ParseInt(row.Ts, 10, 64)
	if err != nil {
		return trading.Fill{}, fmt.Errorf("time: %w", err)
	}
	// OKX reports 0 for a fill that closes nothing, and may report nothing at all.
	var realized decimal.NullDecimal
	if pnl, err := decimal.NewFromString(row.FillPnl); err == nil && !pnl.IsZero() {
		realized = decimal.NewNullDecimal(pnl)
	}
	positionSide := strings.ToLower(row.PosSide)
	if positionSide != "long" && positionSide != "short" {
		positionSide = ""
	}
	quantity := size.Mul(contract)
	return trading.Fill{
		TradeID:         row.TradeID,
		OrderID:         row.OrdID,
		Symbol:          symbol,
		NativeSymbol:    row.InstID,
		Side:            strings.ToLower(row.Side),
		PositionSide:    positionSide,
		Price:           price,
		Quantity:        quantity,
		QuoteQuantity:   price.Mul(quantity),
		Commission:      fee.Neg(),
		CommissionAsset: row.FeeCcy,
		RealizedPnL:     realized,
		IsMaker:         row.ExecType == "M",
		Time:            time.UnixMilli(ts).UTC(),
	}, nil
}
