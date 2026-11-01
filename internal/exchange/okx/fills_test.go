package okx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
	"vergex/internal/trading"
)

// fillRow is one row as OKX sends it.
func fillRow(bill, trade, inst, side, posSide, size, fee, pnl string) string {
	return fmt.Sprintf(`{"instId":%q,"tradeId":%q,"ordId":"order-%s","billId":%q,"side":%q,"posSide":%q,"fillPx":"100.5","fillSz":%q,"fillPnl":%q,"fee":%q,"feeCcy":"USDT","ts":"1700000000123","execType":"M"}`,
		inst, trade, trade, bill, side, posSide, size, pnl, fee)
}

// fillsClient answers fills-history from rows and the instrument list from contracts,
// and keeps the query of each read of fills.
func fillsClient(t *testing.T, contracts func(instID string) (string, error), rows func(query map[string]string) (string, error)) (*OKXTrader, *[]map[string]string) {
	t.Helper()
	asked := &[]map[string]string{}
	client := NewAccountTrader("key", "secret", "pass")
	client.httpClient = &http.Client{Transport: executionTransport(func(r *http.Request) (*http.Response, error) {
		var body string
		var err error
		switch r.URL.Path {
		case okxInstrumentsPath:
			body, err = contracts(r.URL.Query().Get("instId"))
		case "/api/v5/trade/fills-history":
			query := map[string]string{}
			for key := range r.URL.Query() {
				query[key] = r.URL.Query().Get(key)
			}
			*asked = append(*asked, query)
			body, err = rows(query)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return client, asked
}

func okxData(data string) string { return `{"code":"0","data":` + data + `}` }

// onlyBTC knows one contract, as OKX answers for one it no longer describes.
func onlyBTC(instID string) (string, error) {
	if instID == "BTC-USDT-SWAP" {
		return okxData(btcContract), nil
	}
	return okxData("[]"), nil
}

func someRows(rows ...string) func(map[string]string) (string, error) {
	return func(map[string]string) (string, error) { return okxData("[" + strings.Join(rows, ",") + "]"), nil }
}

const btcContract = `[{"instId":"BTC-USDT-SWAP","ctVal":"0.01","ctMult":"1","lotSz":"1","minSz":"1","tickSz":"0.1","ctType":"linear"}]`

func TestAFillIsReadInBaseUnitsWithItsCostAndPnL(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	client, asked := fillsClient(t, onlyBTC, someRows(
		fillRow("3", "t3", "BTC-USDT-SWAP", "sell", "long", "25", "-0.02", "12.5"),
		fillRow("2", "t2", "BTC-USDT-SWAP", "buy", "net", "3", "0.01", "0"),
		fillRow("1", "t1", "BTC-USDT-SWAP", "buy", "long", "3", "-0.01", ""),
	))
	fills, err := client.FillsSince(trading.FillCursor{Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 3 {
		t.Fatalf("fills = %+v", fills)
	}
	closing, net, open := fills[0], fills[1], fills[2]

	// 25 contracts of 0.01 BTC.
	if closing.Quantity.String() != "0.25" || closing.QuoteQuantity.String() != "25.125" || closing.Price.String() != "100.5" {
		t.Errorf("sizes = %+v", closing)
	}
	if closing.Symbol != "BTCUSDT" || closing.NativeSymbol != "BTC-USDT-SWAP" || closing.OrderID != "order-t3" || closing.TradeID != "t3" || !closing.IsMaker {
		t.Errorf("names = %+v", closing)
	}
	if closing.Side != "sell" || closing.PositionSide != "long" {
		t.Errorf("sides = %+v", closing)
	}
	if !closing.RealizedPnL.Valid || closing.RealizedPnL.Decimal.String() != "12.5" {
		t.Errorf("realized = %+v", closing.RealizedPnL)
	}
	// OKX reports a cost as negative and a rebate as positive.
	if closing.Commission.String() != "0.02" {
		t.Errorf("cost = %s", closing.Commission)
	}
	if net.Commission.String() != "-0.01" {
		t.Errorf("rebate = %s", net.Commission)
	}
	// One-way mode names no position side, and a fill that closes nothing reports 0.
	if net.PositionSide != "" || net.RealizedPnL.Valid {
		t.Errorf("net = %+v", net)
	}
	// Nothing reported at all reads the same way.
	if open.RealizedPnL.Valid {
		t.Errorf("open = %+v", open)
	}

	query := (*asked)[0]
	if query["instType"] != "SWAP" || query["limit"] != strconv.Itoa(fillBatch) || query["begin"] != strconv.FormatInt(since.UnixMilli(), 10) || query["after"] != "" {
		t.Errorf("query = %v", query)
	}
}

func TestReadingPagesBackUntilThereAreNoMore(t *testing.T) {
	page := func(from, count int) string {
		rows := make([]string, 0, count)
		for i := range count {
			id := strconv.Itoa(from - i)
			rows = append(rows, fillRow(id, "t"+id, "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "0"))
		}
		return "[" + strings.Join(rows, ",") + "]"
	}
	client, asked := fillsClient(t, onlyBTC, func(query map[string]string) (string, error) {
		switch query["after"] {
		case "":
			return okxData(page(250, fillBatch)), nil
		case "151":
			return okxData(page(150, fillBatch)), nil
		case "51":
			return okxData(page(50, 50)), nil
		}
		t.Errorf("after = %q", query["after"])
		return okxData("[]"), nil
	})
	fills, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(fills) != 250 || len(*asked) != 3 {
		t.Fatalf("fills = %d in %d reads", len(fills), len(*asked))
	}
	seen := map[string]bool{}
	for _, fill := range fills {
		seen[fill.TradeID] = true
	}
	if len(seen) != 250 {
		t.Fatalf("%d different trades", len(seen))
	}
	for _, query := range *asked {
		if query["begin"] != (*asked)[0]["begin"] {
			t.Errorf("begin moved: %v", *asked)
		}
	}
}

func TestReadingStopsAtWhatOKXStillKeeps(t *testing.T) {
	client, asked := fillsClient(t, onlyBTC, someRows())
	if _, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-200 * 24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	begin, _ := strconv.ParseInt((*asked)[0]["begin"], 10, 64)
	if begin < time.Now().Add(-historyLimit-time.Minute).UnixMilli() {
		t.Errorf("begin = %d", begin)
	}
}

func TestAContractOKXNoLongerDescribesHoldsNothingUp(t *testing.T) {
	good := fillRow("2", "t2", "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "0")
	gone := fillRow("1", "t1", "OLD-USDT-SWAP", "sell", "long", "7", "-0.01", "0")
	for name, answer := range map[string]string{
		"an empty list": okxData("[]"),
		"OKX's code":    `{"code":"51001","msg":"Instrument ID, Instrument ID code or Spread ID doesn't exist.","data":[]}`,
		"no value":      okxData(`[{"instId":"OLD-USDT-SWAP","ctVal":"","lotSz":"1","minSz":"1","tickSz":"0.1"}]`),
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := fillsClient(t, func(instID string) (string, error) {
				if instID == "BTC-USDT-SWAP" {
					return okxData(btcContract), nil
				}
				return answer, nil
			}, someRows(good, gone))
			fills, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-time.Hour)})
			if err != nil {
				t.Fatal(err)
			}
			if len(fills) != 2 || fills[0].Unbookable != "" || fills[0].Quantity.String() != "0.01" {
				t.Fatalf("fills = %+v", fills)
			}
			skipped := fills[1]
			if skipped.Unbookable == "" || skipped.TradeID != "t1" || skipped.NativeSymbol != "OLD-USDT-SWAP" || skipped.Time.IsZero() {
				t.Fatalf("skipped = %+v", skipped)
			}
			// It says how much in OKX's own units, and never claims a base size.
			if !strings.Contains(skipped.Unbookable, "sell 7 contracts") || !skipped.Quantity.IsZero() || !skipped.Price.IsZero() {
				t.Fatalf("skipped = %+v", skipped)
			}
		})
	}
}

func TestAReadThatFailedReportsNothing(t *testing.T) {
	good := fillRow("2", "t2", "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "0")
	other := fillRow("1", "t1", "ETH-USDT-SWAP", "buy", "long", "1", "-0.01", "0")
	for name, answer := range map[string]func() (string, error){
		"the network": func() (string, error) { return "", errors.New("connection reset") },
		"a rate limit": func() (string, error) {
			return `{"code":"50011","msg":"Too Many Requests","data":[]}`, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := fillsClient(t, func(instID string) (string, error) {
				if instID == "BTC-USDT-SWAP" {
					return okxData(btcContract), nil
				}
				return answer()
			}, someRows(good, other))
			fills, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-time.Hour)})
			if err == nil || len(fills) != 0 {
				t.Fatalf("fills = %+v, err %v", fills, err)
			}
		})
	}
}

func fullPage(from int) []string {
	rows := make([]string, 0, fillBatch)
	for i := range fillBatch {
		id := strconv.Itoa(from - i)
		rows = append(rows, fillRow(id, "t"+id, "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "0"))
	}
	return rows
}

func TestALaterPageThatFailsReportsNothing(t *testing.T) {
	client, _ := fillsClient(t, onlyBTC, func(query map[string]string) (string, error) {
		if query["after"] == "" {
			return okxData("[" + strings.Join(fullPage(1000), ",") + "]"), nil
		}
		return "", errors.New("connection reset")
	})
	fills, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-time.Hour)})
	if err == nil || len(fills) != 0 {
		t.Fatalf("fills = %d, err %v", len(fills), err)
	}
}

func TestARowThatCanNeverBeReadHoldsNothingUp(t *testing.T) {
	client, _ := fillsClient(t, onlyBTC, someRows(
		fillRow("2", "t2", "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "0"),
		fillRow("1", "t1", "BTC-USDT-SWAP", "buy", "long", "oops", "-0.01", "0"),
	))
	fills, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-time.Hour)})
	if err != nil || len(fills) != 2 || fills[0].Unbookable != "" || fills[1].Unbookable == "" {
		t.Fatalf("fills = %+v, err %v", fills, err)
	}
}

func TestAFullPageWithNoBillIDCannotBeReadPast(t *testing.T) {
	rows := fullPage(1000)
	for i := range rows {
		rows[i] = fillRow("", "t"+strconv.Itoa(i), "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "0")
	}
	client, _ := fillsClient(t, onlyBTC, someRows(rows...))
	fills, err := client.FillsSince(trading.FillCursor{Since: time.Now().Add(-time.Hour)})
	if err == nil || len(fills) != 0 {
		t.Fatalf("fills = %d, err %v", len(fills), err)
	}
}

// The row type matches what the docs show, so a rename would show up here.
func TestAFillRowDecodesEveryFieldItUses(t *testing.T) {
	var row okxFill
	if err := json.Unmarshal([]byte(fillRow("9", "t9", "BTC-USDT-SWAP", "buy", "long", "1", "-0.01", "2")), &row); err != nil {
		t.Fatal(err)
	}
	if row.BillID != "9" || row.TradeID != "t9" || row.OrdID != "order-t9" || row.FillPnl != "2" || row.ExecType != "M" || row.Ts != "1700000000123" {
		t.Fatalf("row = %+v", row)
	}
}
