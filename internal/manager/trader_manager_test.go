package manager

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"vergex/internal/store"
	"vergex/internal/trader"
)

// newIdleTrader returns a zero-value AutoTrader. It is safe to store in the
// manager map for map-semantics tests: GetStatus works on a zero value and
// Stop returns early because the trader is not running. It must NOT be used
// for anything that touches an exchange (Run, GetAccountInfo, ...).
func newIdleTrader() *trader.AutoTrader {
	return &trader.AutoTrader{}
}

// insertTrader places a trader directly into the manager's internal map,
// bypassing store loading (same-package access).
func insertTrader(tm *TraderManager, id string, t *trader.AutoTrader) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.traders[id] = t
}

func TestNewTraderManager(t *testing.T) {
	tm := NewTraderManager()

	if tm == nil {
		t.Fatal("NewTraderManager() returned nil")
	}
	if tm.traders == nil {
		t.Error("traders map should be initialized, got nil")
	}
	if len(tm.traders) != 0 {
		t.Errorf("traders map should be empty, got %d entries", len(tm.traders))
	}
	if tm.loadErrors == nil {
		t.Error("loadErrors map should be initialized, got nil")
	}
	if len(tm.loadErrors) != 0 {
		t.Errorf("loadErrors map should be empty, got %d entries", len(tm.loadErrors))
	}
}

func TestGetTrader(t *testing.T) {
	tm := NewTraderManager()

	t.Run("missing ID returns error", func(t *testing.T) {
		got, err := tm.GetTrader("does-not-exist")
		if err == nil {
			t.Fatal("GetTrader on missing ID expected error, got nil")
		}
		if got != nil {
			t.Errorf("GetTrader on missing ID should return nil trader, got %v", got)
		}
		if !strings.Contains(err.Error(), "does-not-exist") {
			t.Errorf("error %q should mention the trader ID", err.Error())
		}
	})

	t.Run("existing ID returns same instance", func(t *testing.T) {
		at := newIdleTrader()
		insertTrader(tm, "trader-1", at)

		got, err := tm.GetTrader("trader-1")
		if err != nil {
			t.Fatalf("GetTrader unexpected error: %v", err)
		}
		if got != at {
			t.Errorf("GetTrader returned %p, want the stored instance %p", got, at)
		}
	})
}

func TestGetLoadError(t *testing.T) {
	tm := NewTraderManager()

	t.Run("unknown trader returns nil", func(t *testing.T) {
		if err := tm.GetLoadError("unknown"); err != nil {
			t.Errorf("GetLoadError for unknown trader = %v, want nil", err)
		}
	})

	t.Run("stored error is returned", func(t *testing.T) {
		wantErr := errors.New("failed to create trader: boom")
		tm.mu.Lock()
		tm.loadErrors["trader-x"] = wantErr
		tm.mu.Unlock()

		if got := tm.GetLoadError("trader-x"); !errors.Is(got, wantErr) {
			t.Errorf("GetLoadError = %v, want %v", got, wantErr)
		}
	})
}

func TestGetAllTradersReturnsCopy(t *testing.T) {
	tm := NewTraderManager()
	at1 := newIdleTrader()
	at2 := newIdleTrader()
	insertTrader(tm, "t1", at1)
	insertTrader(tm, "t2", at2)

	all := tm.GetAllTraders()

	if len(all) != 2 {
		t.Fatalf("GetAllTraders returned %d entries, want 2", len(all))
	}
	if all["t1"] != at1 || all["t2"] != at2 {
		t.Error("GetAllTraders should return the same trader instances")
	}

	// Mutating the returned map must not affect internal state.
	delete(all, "t1")
	all["t3"] = newIdleTrader()

	if _, err := tm.GetTrader("t1"); err != nil {
		t.Errorf("deleting from returned map leaked into internal state: %v", err)
	}
	if _, err := tm.GetTrader("t3"); err == nil {
		t.Error("adding to returned map leaked into internal state")
	}
	if got := len(tm.GetTraderIDs()); got != 2 {
		t.Errorf("internal trader count = %d after mutating returned map, want 2", got)
	}
}

func TestGetTraderIDs(t *testing.T) {
	tm := NewTraderManager()

	t.Run("empty manager returns empty non-nil slice", func(t *testing.T) {
		ids := tm.GetTraderIDs()
		if ids == nil {
			t.Fatal("GetTraderIDs should return an empty slice, got nil")
		}
		if len(ids) != 0 {
			t.Errorf("GetTraderIDs = %v, want empty", ids)
		}
	})

	t.Run("returns all IDs", func(t *testing.T) {
		want := []string{"a", "b", "c"}
		for _, id := range want {
			insertTrader(tm, id, newIdleTrader())
		}

		got := tm.GetTraderIDs()
		sort.Strings(got)
		if len(got) != len(want) {
			t.Fatalf("GetTraderIDs returned %d IDs, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("GetTraderIDs[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
}

func TestRemoveTrader(t *testing.T) {
	t.Run("removes existing non-running trader", func(t *testing.T) {
		tm := NewTraderManager()
		insertTrader(tm, "t1", newIdleTrader())

		tm.RemoveTrader("t1")

		if _, err := tm.GetTrader("t1"); err == nil {
			t.Error("trader t1 should be removed")
		}
		if got := len(tm.GetTraderIDs()); got != 0 {
			t.Errorf("trader count after removal = %d, want 0", got)
		}
	})

	t.Run("missing ID is a no-op", func(t *testing.T) {
		tm := NewTraderManager()
		insertTrader(tm, "t1", newIdleTrader())

		tm.RemoveTrader("missing") // must not panic

		if _, err := tm.GetTrader("t1"); err != nil {
			t.Errorf("unrelated trader was removed: %v", err)
		}
	})
}

func TestStartAllEmpty(t *testing.T) {
	tm := NewTraderManager()
	tm.StartAll() // must not panic with no traders
}

func TestStopAllWithIdleTraders(t *testing.T) {
	tm := NewTraderManager()
	tm.StopAll() // empty: must not panic

	insertTrader(tm, "t1", newIdleTrader())
	insertTrader(tm, "t2", newIdleTrader())
	tm.StopAll() // not-running traders: Stop is an early-return no-op
}

func TestTraderLogTag(t *testing.T) {
	tests := []struct {
		name       string
		traderID   string
		traderName string
		want       string
	}{
		{
			name:       "with name",
			traderID:   "abc-123",
			traderName: "MyBot",
			want:       "[trader_id=abc-123 trader_name=MyBot]",
		},
		{
			name:     "without name",
			traderID: "abc-123",
			want:     "[trader_id=abc-123]",
		},
		{
			name: "both empty",
			want: "[trader_id=]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := traderLogTag(tt.traderID, tt.traderName); got != tt.want {
				t.Errorf("traderLogTag(%q, %q) = %q, want %q", tt.traderID, tt.traderName, got, tt.want)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	tm := NewTraderManager()

	const traderCount = 16
	ids := make([]string, 0, traderCount)
	for i := 0; i < traderCount; i++ {
		id := fmt.Sprintf("trader-%d", i)
		ids = append(ids, id)
		insertTrader(tm, id, newIdleTrader())
	}

	const (
		goroutinesPerKind = 8
		iterations        = 200
	)

	var wg sync.WaitGroup

	// Readers: GetTrader / GetLoadError
	for g := 0; g < goroutinesPerKind; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				id := ids[(seed+i)%traderCount]
				_, _ = tm.GetTrader(id)
				_ = tm.GetLoadError(id)
			}
		}(g)
	}

	// Readers: GetAllTraders / GetTraderIDs
	for g := 0; g < goroutinesPerKind; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = tm.GetAllTraders()
				_ = tm.GetTraderIDs()
			}
		}()
	}

	// Writers: RemoveTrader (including repeated removal of the same ID)
	for g := 0; g < goroutinesPerKind; g++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tm.RemoveTrader(ids[(seed+i)%traderCount])
			}
		}(g)
	}

	wg.Wait()

	if got := len(tm.GetTraderIDs()); got != 0 {
		t.Errorf("all traders should be removed after concurrent removal, %d left", got)
	}
}

func TestEnsureUserTradersLoadedBacksOffAfterFailure(t *testing.T) {
	st, err := store.New(t.TempDir() + "/vergex.db")
	if err != nil {
		t.Fatalf("store.New failed: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	const userID = "user-1"
	db := st.GormDB()
	if err := db.Create(&store.AIModel{ID: "model-1", UserID: userID, Name: "DeepSeek", Provider: "deepseek", Enabled: true}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
	if err := db.Create(&store.Exchange{ID: "exchange-1", UserID: userID, ExchangeType: "binance", Name: "Binance", Type: "cex", Enabled: true}).Error; err != nil {
		t.Fatalf("create exchange: %v", err)
	}
	// A missing strategy fails the load before anything reaches the exchange.
	if err := st.Trader().Create(&store.Trader{ID: "trader-1", UserID: userID, Name: "bot", AIModelID: "model-1", ExchangeID: "exchange-1", StrategyID: "missing"}); err != nil {
		t.Fatalf("create trader: %v", err)
	}

	tm := NewTraderManager()
	now := time.Unix(1_800_000_000, 0)
	tm.now = func() time.Time { return now }
	failedAt := func() time.Time {
		tm.mu.RLock()
		defer tm.mu.RUnlock()
		return tm.loadFailedAt["trader-1"]
	}

	_ = tm.EnsureUserTradersLoaded(st, userID)
	if tm.GetLoadError("trader-1") == nil || !failedAt().Equal(now) {
		t.Fatalf("first load should fail and be recorded, got err=%v at %v", tm.GetLoadError("trader-1"), failedAt())
	}
	first := now

	now = now.Add(loadRetryInterval / 2)
	_ = tm.EnsureUserTradersLoaded(st, userID)
	if !failedAt().Equal(first) {
		t.Fatalf("passive load retried within %v", loadRetryInterval)
	}

	_ = tm.LoadUserTradersFromStore(st, userID)
	if !failedAt().Equal(now) {
		t.Fatal("explicit load should retry immediately")
	}

	now = now.Add(loadRetryInterval)
	_ = tm.EnsureUserTradersLoaded(st, userID)
	if !failedAt().Equal(now) {
		t.Fatal("passive load should retry once the interval has passed")
	}
}
