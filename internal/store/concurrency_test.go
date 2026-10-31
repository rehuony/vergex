package store

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vergex/internal/testdb"
)

// runTogether runs fn in several goroutines at once and reports each error.
func runTogether(t *testing.T, fn func() error) {
	t.Helper()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestRecordingTheSameOrderAndFillAtOnceKeepsOneRow(t *testing.T) {
	store := NewOrderStore(testdb.Open(t))
	if err := store.InitTables(); err != nil {
		t.Fatal(err)
	}
	runTogether(t, func() error {
		order := &TraderOrder{TraderID: "t1", ExchangeID: "e1", ExchangeOrderID: "o1", Symbol: "BTCUSDT", Side: "BUY"}
		if err := store.CreateOrder(order); err != nil {
			return err
		}
		if order.ID == 0 {
			t.Error("order has no id")
		}
		return store.CreateFill(&TraderFill{TraderID: "t1", ExchangeID: "e1", OrderID: order.ID, ExchangeOrderID: "o1", ExchangeTradeID: "f1", Symbol: "BTCUSDT", Side: "BUY"})
	})
	var orders, fills int64
	store.db.Model(&TraderOrder{}).Count(&orders)
	store.db.Model(&TraderFill{}).Count(&fills)
	if orders != 1 || fills != 1 {
		t.Fatalf("orders = %d, fills = %d", orders, fills)
	}
}

func TestRecordingTheSamePositionAtOnceKeepsOneRow(t *testing.T) {
	store := NewPositionStore(testdb.Open(t))
	if err := store.InitTables(); err != nil {
		t.Fatal(err)
	}
	entry := time.Now().UnixMilli()
	runTogether(t, func() error {
		return store.CreateOpenPosition(&TraderPosition{TraderID: "t1", ExchangeID: "e1", ExchangePositionID: "p1", Symbol: "BTCUSDT", Side: "LONG", Quantity: 1, EntryPrice: 100, EntryTime: entry})
	})
	var rows int64
	store.db.Model(&TraderPosition{}).Count(&rows)
	if rows != 1 {
		t.Fatalf("positions = %d", rows)
	}

	closed := &ClosedPnLRecord{Symbol: "ETHUSDT", Side: "long", Quantity: 1, EntryPrice: 10, ExitPrice: 11, ExitTime: entry, EntryTime: entry, OrderID: "c1"}
	var created atomic.Int32
	runTogether(t, func() error {
		ok, err := store.CreateFromClosedPnL("t1", "e1", "binance", closed)
		if ok {
			created.Add(1)
		}
		return err
	})
	if created.Load() != 1 {
		t.Fatalf("closed positions created = %d", created.Load())
	}
}

func TestDecisionsKeepModelOutputPostgresWouldReject(t *testing.T) {
	store := NewDecisionStore(testdb.Open(t))
	if err := store.initTables(); err != nil {
		t.Fatal(err)
	}
	record := &DecisionRecord{TraderID: "t1", RawResponse: "up\x00 to \xffhere", Success: true}
	if err := store.LogDecision(record); err != nil {
		t.Fatal(err)
	}
	var saved DecisionRecordDB
	if err := store.db.First(&saved, record.ID).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(saved.RawResponse, "\x00") || !strings.HasPrefix(saved.RawResponse, "up to ") {
		t.Fatalf("raw response = %q", saved.RawResponse)
	}
}
