package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
	"vergex/internal/account"

	"github.com/gin-gonic/gin"
	"vergex/internal/database/testdb"
	"vergex/internal/futures"
	"vergex/internal/futures/run"
	"vergex/internal/trader"

	"github.com/google/uuid"
)

type fakeAccountCloser struct {
	positions []map[string]interface{}
	calls     int
	lookups   int
	fail      bool
	last      account.Order
}

func (f *fakeAccountCloser) GetPositions() ([]map[string]interface{}, error) { return f.positions, nil }
func (f *fakeAccountCloser) RouteClientID(id string) (string, error)         { return "broker-" + id, nil }
func (f *fakeAccountCloser) SubmitOrder(_ context.Context, o account.Order) (account.Order, error) {
	f.calls++
	f.last = o
	if f.fail {
		return o, errors.New("timeout")
	}
	o.Status = "FILLED"
	o.Filled = o.Quantity
	o.OrderID = "123"
	return o, nil
}
func (f *fakeAccountCloser) LookupOrder(_ context.Context, o account.Order) (account.Order, error) {
	f.lookups++
	return o, errors.New("unconfirmed")
}

const (
	firstAccount   = "00000000-0000-4000-8000-0000000000f1"
	secondAccount  = "00000000-0000-4000-8000-0000000000f2"
	foreignAccount = "00000000-0000-4000-8000-0000000000f3"
)

func closeFixture(t *testing.T) (*Server, *fakeAccountCloser, accountCloseRequest) {
	t.Helper()
	st := NewStores(testdb.Open(t))
	db := st.DB
	testdb.AddUser(t, st.DB)
	testdb.AddOtherUser(t, st.DB)
	for _, ex := range []account.Account{
		{ID: firstAccount, UserID: testdb.UserID, Venue: "binance", APIKey: "first-key"},
		{ID: secondAccount, UserID: testdb.UserID, Venue: "binance", APIKey: "second-key"},
		{ID: foreignAccount, UserID: testdb.OtherUserID, Venue: "binance", APIKey: "foreign-key"},
	} {
		if e := db.Create(&ex).Error; e != nil {
			t.Fatal(e)
		}
	}
	f := &fakeAccountCloser{positions: []map[string]interface{}{{"symbol": "BTCUSDT", "side": "short", "positionAmt": -0.019, "entryPrice": 81798.3}}}
	in := accountCloseRequest{AccountID: secondAccount, RequestID: uuid.NewString(), Symbol: "BTCUSDT", Side: "short", Quantity: "0.019", Entry: "81798.3"}
	return &Server{stores: st}, f, in
}
func TestAccountCloseUsesSelectedBinanceWithoutAutopilot(t *testing.T) {
	s, f, in := closeFixture(t)
	factory := func(ex *account.Account) (accountCloser, error) {
		if ex.ID != secondAccount {
			t.Fatalf("wrong account: %s", ex.ID)
		}
		return f, nil
	}
	order, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory)
	if err != nil || order.Status != "FILLED" {
		t.Fatal(order, err)
	}
	if f.calls != 1 || !f.last.ReduceOnly || f.last.Type != "market" || f.last.Side != "long" || f.last.Quantity != "0.019" || f.last.VenueClientID == "" {
		t.Fatal(f.last)
	}
	var count int64
	s.stores.DB.Model(&trader.Trader{}).Count(&count)
	if count != 0 {
		t.Fatal("close created an Autopilot")
	}
	// Retransmission returns the persisted result, without sending a second order.
	if _, err = s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory); err != nil || f.calls != 1 {
		t.Fatal(err, f.calls)
	}
}
func TestAccountCloseBlocksForeignChangedAndRunningAccounts(t *testing.T) {
	for _, name := range []string{foreignAccount, "missing", "increased", "new-entry", "running", "no-position", "wrong-side"} {
		t.Run(name, func(t *testing.T) {
			s, f, in := closeFixture(t)
			switch name {
			case foreignAccount:
				in.AccountID = foreignAccount
			case "missing":
				in.AccountID = ""
			case "increased":
				f.positions[0]["positionAmt"] = -0.02
			case "new-entry":
				f.positions[0]["entryPrice"] = 81800.0
			case "running":
				testdb.AddTrader(t, s.stores.DB, &testdb.Trader{AccountID: secondAccount, IsRunning: true})
			case "no-position":
				f.positions = nil
			case "wrong-side":
				in.Side = "long"
			}
			_, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, func(*account.Account) (accountCloser, error) { return f, nil })
			if err == nil || f.calls != 0 {
				t.Fatal("unsafe close", err, f.calls)
			}
		})
	}
}
func TestAccountCloseTimeoutCannotRepeatWithNewRequestID(t *testing.T) {
	s, f, in := closeFixture(t)
	f.fail = true
	factory := func(*account.Account) (accountCloser, error) { return f, nil }
	if _, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory); err == nil {
		t.Fatal("timeout hidden")
	}
	in.RequestID = uuid.NewString()
	if _, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory); err == nil || f.calls != 1 || f.lookups != 1 {
		t.Fatal("ambiguous order repeated", err, f.calls, f.lookups)
	}
}
func TestAccountCloseWaitsForScheduledAndUnknownEntries(t *testing.T) {
	for _, status := range []string{"scheduled", "submitting", "NEW", "UNKNOWN", "PARTIALLY_FILLED"} {
		t.Run(status, func(t *testing.T) {
			s, f, in := closeFixture(t)
			ex, _ := s.stores.Accounts.GetByID(testdb.UserID, secondAccount)
			db := s.stores.DB
			now := time.Now().UTC()
			if e := db.Create(&futures.Plan{UserID: testdb.UserID, ID: "plan", AccountID: secondAccount, Venue: "binance", Symbol: "BTCUSDT", Status: "approved", Body: "{}"}).Error; e != nil {
				t.Fatal(e)
			}
			run := run.Run{ID: uuid.NewString(), UserID: testdb.UserID, AccountID: secondAccount, AccountKey: ex.Identity, PlanID: "plan", PlanHash: "h", ApprovalHash: "h", State: "stopping", ApprovedAt: now, UpdatedAt: now}
			if e := db.Create(&run).Error; e != nil {
				t.Fatal(e)
			}
			if e := db.Exec(`INSERT INTO futures.run_orders (run_id, seq, role, symbol, side, client_id, status) VALUES (?, 0, 'entry', 'BTCUSDT', 'long', 'c', ?)`, run.ID, status).Error; e != nil {
				t.Fatal(e)
			}
			_, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, func(*account.Account) (accountCloser, error) { return f, nil })
			if err == nil || f.calls != 0 {
				t.Fatal("entry can reopen position", status, err)
			}
		})
	}
}

func TestAccountCloseSupportsOKXWithoutAutopilot(t *testing.T) {
	for _, venue := range []string{"okx"} {
		t.Run(venue, func(t *testing.T) {
			s, f, in := closeFixture(t)
			if e := s.stores.DB.Model(&account.Account{}).Where("id = ?", secondAccount).Update("venue", venue).Error; e != nil {
				t.Fatal(e)
			}
			if venue == "okx" {
				in.Symbol = "BTC-USDT-SWAP"
				f.positions[0]["symbol"] = in.Symbol
			}
			factory := func(ex *account.Account) (accountCloser, error) {
				if ex.ID != secondAccount || ex.Venue != venue {
					t.Fatal("wrong account")
				}
				return f, nil
			}
			order, e := s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory)
			if e != nil || order.Status != "FILLED" || !f.last.ReduceOnly || f.last.Symbol != in.Symbol {
				t.Fatalf("close %+v %v", order, e)
			}
			if _, e = s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory); e != nil || f.calls != 1 {
				t.Fatal("duplicate close", e)
			}
			ex, e := s.futuresWorkspace(testdb.UserID, secondAccount, venue)
			if e != nil || ex.Venue != venue {
				t.Fatal("workspace rejected", e)
			}
		})
	}
}

func TestAccountCloseAcceptsWeightedAverageWithoutRounding(t *testing.T) {
	for _, venue := range []string{"binance", "okx"} {
		t.Run(venue, func(t *testing.T) {
			s, f, in := closeFixture(t)
			s.stores.DB.Model(&account.Account{}).Where("id = ?", secondAccount).Update("venue", venue)
			in.Symbol, in.Quantity, in.Entry = "QUSDT", "12257", "0.1287878419678551"
			if venue == "okx" {
				in.Symbol = "Q-USDT-SWAP"
			}
			f.positions = []map[string]interface{}{{"symbol": in.Symbol, "side": "short", "positionAmt": float64(-12257), "entryPrice": 0.1287878419678551}}
			o, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, func(*account.Account) (accountCloser, error) { return f, nil })
			if err != nil || f.calls != 1 || o.Price != account.Decimal(in.Entry) || o.Quantity != "12257" || !o.ReduceOnly {
				t.Fatal(o, err, f.calls)
			}
			if !account.ValidOrderPrice(o) {
				t.Fatal("adapter would reject weighted entry")
			}
		})
	}
}

func TestAccountCloseErrorsDistinguishNotSentFromUnconfirmed(t *testing.T) {
	for _, scenario := range []string{"invalid", "changed", "timeout", "previous-timeout"} {
		t.Run(scenario, func(t *testing.T) {
			s, f, in := closeFixture(t)
			factory := func(*account.Account) (accountCloser, error) { return f, nil }
			switch scenario {
			case "invalid":
				in.Quantity = "0"
			case "changed":
				f.positions[0]["entryPrice"] = 82000.0
			case "timeout":
				f.fail = true
			case "previous-timeout":
				f.fail = true
				s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory)
				in.RequestID = uuid.NewString()
			}
			_, err := s.closeFuturesAccount(context.Background(), testdb.UserID, in, factory)
			if err == nil {
				t.Fatal("expected close error")
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			writeAccountCloseError(c, err)
			var result map[string]string
			if json.Unmarshal(w.Body.Bytes(), &result) != nil {
				t.Fatal(w.Body.String())
			}
			expected := "unconfirmed"
			if scenario == "invalid" || scenario == "changed" {
				expected = "not_submitted"
				if f.calls != 0 {
					t.Fatal("unexpected submission")
				}
			}
			if result["close_state"] != expected {
				t.Fatal(result)
			}
			if scenario == "previous-timeout" && f.calls != 1 {
				t.Fatal("duplicate submission")
			}
		})
	}
}
