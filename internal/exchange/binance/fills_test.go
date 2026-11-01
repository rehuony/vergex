package binance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"vergex/internal/trading"
)

// fillServer answers what reading fills asks of Binance, and keeps what it was asked.
type fillServer struct {
	mu      sync.Mutex
	income  func(kind string, start int64) []map[string]any
	trades  func(q map[string]string) (string, int)
	asked   []map[string]string
	symbols []string
}

func (f *fillServer) trader(t *testing.T) *FuturesTrader {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		query := map[string]string{}
		for key := range r.Form {
			query[key] = r.Form.Get(key)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/fapi/v1/income":
			var rows []map[string]any
			if f.income != nil {
				start, _ := strconv.ParseInt(query["startTime"], 10, 64)
				rows = f.income(query["incomeType"], start)
			}
			if rows == nil {
				rows = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(rows)
		case "/fapi/v1/userTrades":
			f.mu.Lock()
			f.asked = append(f.asked, query)
			f.symbols = append(f.symbols, query["symbol"])
			f.mu.Unlock()
			body, status := `[]`, http.StatusOK
			if f.trades != nil {
				body, status = f.trades(query)
			}
			w.WriteHeader(status)
			fmt.Fprint(w, body)
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	t.Cleanup(s.Close)
	trader := NewExecutionTrader("test-key", "test-secret")
	trader.client.BaseURL = s.URL
	trader.client.HTTPClient = s.Client()
	return trader
}

const tradeFixture = `[{"id":101,"orderId":77,"symbol":"BTCUSDT","side":"SELL","positionSide":"LONG","price":"0.000000541","qty":"12.5",
	"quoteQty":"0.0000067625","commission":"0.0012","commissionAsset":"BNB","realizedPnl":"-1.25","maker":true,"buyer":false,"time":1700000000123}]`

func TestAClosedSymbolOnlyTheLedgerStillHasOpenIsFound(t *testing.T) {
	// No income and no position on the exchange: only the ledger knows to ask.
	server := &fillServer{trades: func(map[string]string) (string, int) { return tradeFixture, http.StatusOK }}
	since := time.Now().Add(-72 * time.Hour)
	fills, err := server.trader(t).FillsSince(trading.FillCursor{Since: since, OpenSymbols: []string{"BTCUSDT"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(server.asked) != 1 {
		t.Fatalf("asked %v", server.asked)
	}
	asked := server.asked[0]
	if asked["symbol"] != "BTCUSDT" || asked["startTime"] != strconv.FormatInt(since.UnixMilli(), 10) || asked["fromID"] != "" || asked["fromId"] != "" {
		t.Fatalf("asked %v", asked)
	}
	if len(fills) != 1 {
		t.Fatalf("fills = %+v", fills)
	}
	fill := fills[0]
	if fill.TradeID != "101" || fill.OrderID != "77" || fill.Side != "sell" || fill.PositionSide != "long" || !fill.IsMaker || fill.CommissionAsset != "BNB" {
		t.Fatalf("fill = %+v", fill)
	}
	// Numbers stay exactly as sent.
	if fill.Price.String() != "0.000000541" || fill.Quantity.String() != "12.5" || fill.QuoteQuantity.String() != "0.0000067625" || fill.Commission.String() != "0.0012" {
		t.Fatalf("fill = %+v", fill)
	}
	if !fill.RealizedPnL.Valid || fill.RealizedPnL.Decimal.String() != "-1.25" {
		t.Fatalf("realized = %+v", fill.RealizedPnL)
	}
}

func TestALongDowntimeIsReadAWeekAtATime(t *testing.T) {
	since := time.Now().Add(-20 * 24 * time.Hour)
	first, second := since.Add(2*24*time.Hour).UnixMilli(), since.Add(10*24*time.Hour).UnixMilli()
	server := &fillServer{
		income: func(kind string, _ int64) []map[string]any {
			return []map[string]any{{"symbol": "ETHUSDT", "time": first, "incomeType": kind}}
		},
		trades: func(q map[string]string) (string, int) {
			start, _ := strconv.ParseInt(q["startTime"], 10, 64)
			end, _ := strconv.ParseInt(q["endTime"], 10, 64)
			switch {
			case start <= first && first <= end:
				return tradeFixture, http.StatusOK
			case start <= second && second <= end:
				return strings.ReplaceAll(tradeFixture, `"id":101`, `"id":102`), http.StatusOK
			}
			return `[]`, http.StatusOK
		},
	}
	fills, err := server.trader(t).FillsSince(trading.FillCursor{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 2 || fills[0].TradeID != "101" || fills[1].TradeID != "102" {
		t.Fatalf("fills = %+v", fills)
	}
	if len(server.asked) != 3 {
		t.Fatalf("asked %v", server.asked)
	}
	for _, asked := range server.asked {
		start, _ := strconv.ParseInt(asked["startTime"], 10, 64)
		end, _ := strconv.ParseInt(asked["endTime"], 10, 64)
		if end-start > tradeWindow.Milliseconds() || end <= start {
			t.Fatalf("asked %v", asked)
		}
	}
}

func TestAFullBatchOfTradesIsReadAgainInHalves(t *testing.T) {
	now := time.Now().UnixMilli()
	since := now - time.Hour.Milliseconds()
	// More trades than one read returns, spread over the hour, of which a read returns
	// the latest: Binance does not say which end it keeps.
	var all []string
	var times []int64
	for i := range fillBatch + 200 {
		at := since + int64(i)*time.Hour.Milliseconds()/int64(fillBatch+200)
		times = append(times, at)
		all = append(all, fmt.Sprintf(`{"id":%d,"orderId":1,"symbol":"BTCUSDT","side":"BUY","price":"1","qty":"1","quoteQty":"1","commission":"0","commissionAsset":"USDT","realizedPnl":"0","time":%d}`, i+1, at))
	}
	server := &fillServer{trades: func(q map[string]string) (string, int) {
		start, _ := strconv.ParseInt(q["startTime"], 10, 64)
		end, _ := strconv.ParseInt(q["endTime"], 10, 64)
		var rows []string
		for i, at := range times {
			if start <= at && at <= end {
				rows = append(rows, all[i])
			}
		}
		if len(rows) > fillBatch {
			rows = rows[len(rows)-fillBatch:]
		}
		return "[" + strings.Join(rows, ",") + "]", http.StatusOK
	}}
	fills, err := server.trader(t).FillsSince(trading.FillCursor{Since: time.UnixMilli(since), OpenSymbols: []string{"BTCUSDT"}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, fill := range fills {
		seen[fill.TradeID] = true
	}
	if len(seen) != fillBatch+200 {
		t.Fatalf("read %d of %d trades", len(seen), fillBatch+200)
	}
}

func TestReadingStopsAtWhatBinanceStillKeeps(t *testing.T) {
	var incomeStart int64
	server := &fillServer{income: func(_ string, start int64) []map[string]any {
		incomeStart = start
		return []map[string]any{{"symbol": "ETHUSDT", "time": start}}
	}}
	if _, err := server.trader(t).FillsSince(trading.FillCursor{Since: time.Now().Add(-200 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	floor := time.Now().Add(-historyLimit - time.Minute).UnixMilli()
	if incomeStart < floor {
		t.Errorf("income asked from %d, before %d", incomeStart, floor)
	}
	if len(server.asked) == 0 {
		t.Fatal("no trades were asked for")
	}
	start, _ := strconv.ParseInt(server.asked[0]["startTime"], 10, 64)
	if start < floor {
		t.Errorf("trades asked from %d, before %d", start, floor)
	}
}

func TestAFullPageOfIncomeIsReadAgainInHalves(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	middle := since.Add(45 * time.Minute).UnixMilli()
	calls := 0
	server := &fillServer{income: func(kind string, start int64) []map[string]any {
		if kind != "COMMISSION" {
			return nil
		}
		calls++
		if calls == 1 {
			// The first read is full, and of only the one symbol.
			rows := make([]map[string]any, incomePage)
			for i := range rows {
				rows[i] = map[string]any{"symbol": "BTCUSDT", "time": middle}
			}
			return rows
		}
		if start > since.UnixMilli() {
			return []map[string]any{{"symbol": "ETHUSDT", "time": middle}}
		}
		return []map[string]any{{"symbol": "BTCUSDT", "time": since.UnixMilli()}}
	}}
	symbols, err := server.trader(t).GetCommissionSymbols(since)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(symbols) != 2 {
		t.Fatalf("calls = %d, symbols = %v", calls, symbols)
	}
}

func TestASymbolThatFailsDoesNotHoldTheOthersUp(t *testing.T) {
	server := &fillServer{trades: func(q map[string]string) (string, int) {
		if q["symbol"] == "ETHUSDT" {
			return `{"code":-1000,"msg":"down"}`, http.StatusBadRequest
		}
		return tradeFixture, http.StatusOK
	}}
	fills, err := server.trader(t).FillsSince(trading.FillCursor{
		Since:       time.Now().Add(-time.Hour),
		OpenSymbols: []string{"BTCUSDT", "ETHUSDT"},
	})
	if err == nil {
		t.Fatal("the failure should be reported")
	}
	if len(fills) != 1 || fills[0].Symbol != "BTCUSDT" {
		t.Fatalf("fills = %+v", fills)
	}
}
