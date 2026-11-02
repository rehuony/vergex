package trader

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/trader/kind"
)

// newIdleTrader returns a zero-value AutoTrader. It is safe to store in the
// manager map for map-semantics tests: GetStatus works on a zero value and
// Stop returns early because the trader is not running. It must NOT be used
// for anything that touches an exchange (Run, GetAccountInfo, ...).
func newIdleTrader() *AutoTrader {
	return &AutoTrader{}
}

// insertTrader places a trader directly into the manager's internal map,
// bypassing store loading (same-package access).
func insertTrader(tm *Manager, id string, t *AutoTrader) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.traders[id] = t
}

func TestNewTraderManager(t *testing.T) {
	tm := NewManager(nil)

	if tm == nil {
		t.Fatal("NewManager(nil) returned nil")
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
	tm := NewManager(nil)

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
	tm := NewManager(nil)

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
	tm := NewManager(nil)
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
	tm := NewManager(nil)

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
		tm := NewManager(nil)
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
		tm := NewManager(nil)
		insertTrader(tm, "t1", newIdleTrader())

		tm.RemoveTrader("missing") // must not panic

		if _, err := tm.GetTrader("t1"); err != nil {
			t.Errorf("unrelated trader was removed: %v", err)
		}
	})
}

func TestStopAllWithIdleTraders(t *testing.T) {
	tm := NewManager(nil)
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
	tm := NewManager(nil)

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
	st := NewStores(testdb.Open(t))

	userID := testdb.AddUser(t, st.DB)
	db := st.DB
	modelID := uuid.NewString()
	if err := db.Create(&agent.Model{ID: modelID, UserID: userID, Name: "DeepSeek", Provider: "deepseek", Enabled: true}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
	accountID := uuid.NewString()
	if err := db.Create(&account.Account{ID: accountID, UserID: userID, Venue: "binance"}).Error; err != nil {
		t.Fatalf("create exchange: %v", err)
	}
	// A config that does not parse fails the load before anything reaches the exchange.
	const traderID = "00000000-0000-4000-8000-0000000000a1"
	if err := st.Traders.Create(&Trader{ID: traderID, UserID: userID, Name: "bot", AgentModelID: modelID, AccountID: accountID, Kind: "agent", Config: []byte(`{"coin_source": 1}`)}); err != nil {
		t.Fatalf("create trader: %v", err)
	}

	tm := NewManager(st)
	now := time.Unix(1_800_000_000, 0)
	tm.now = func() time.Time { return now }
	failedAt := func() time.Time {
		tm.mu.RLock()
		defer tm.mu.RUnlock()
		return tm.loadFailedAt[traderID]
	}

	_ = tm.EnsureUserTradersLoaded(userID)
	if tm.GetLoadError(traderID) == nil || !failedAt().Equal(now) {
		t.Fatalf("first load should fail and be recorded, got err=%v at %v", tm.GetLoadError(traderID), failedAt())
	}
	first := now

	now = now.Add(loadRetryInterval / 2)
	_ = tm.EnsureUserTradersLoaded(userID)
	if !failedAt().Equal(first) {
		t.Fatalf("passive load retried within %v", loadRetryInterval)
	}

	_ = tm.LoadUserTradersFromStore(userID)
	if !failedAt().Equal(now) {
		t.Fatal("explicit load should retry immediately")
	}

	now = now.Add(loadRetryInterval)
	_ = tm.EnsureUserTradersLoaded(userID)
	if !failedAt().Equal(now) {
		t.Fatal("passive load should retry once the interval has passed")
	}
}

// addLoadableTrader stores a trader, with its model and account, for a load to build.
func addLoadableTrader(t *testing.T, st *Stores, userID string) string {
	t.Helper()
	modelID := uuid.NewString()
	if err := st.DB.Create(&agent.Model{ID: modelID, UserID: userID, Name: "DeepSeek", Provider: "deepseek", Enabled: true}).Error; err != nil {
		t.Fatalf("create model: %v", err)
	}
	accountID := uuid.NewString()
	if err := st.DB.Create(&account.Account{ID: accountID, UserID: userID, Venue: "binance"}).Error; err != nil {
		t.Fatalf("create exchange: %v", err)
	}
	config, err := json.Marshal(kind.DefaultAgent())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	traderID := uuid.NewString()
	if err := st.Traders.Create(&Trader{ID: traderID, UserID: userID, Name: "bot", AgentModelID: modelID, AccountID: accountID, Kind: "agent", Config: config}); err != nil {
		t.Fatalf("create trader: %v", err)
	}
	return traderID
}

// awaitBuild waits until a load running in the background has reached the exchange.
func awaitBuild(t *testing.T, entered <-chan struct{}, loaded <-chan error) {
	t.Helper()
	select {
	case <-entered:
	case err := <-loaded:
		t.Fatalf("load finished without building the trader: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("load did not reach the exchange")
	}
}

// finishes fails the test if fn is still blocked after a generous wait.
func finishes(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s blocked behind a load that is waiting for the exchange", what)
	}
}

func TestBlockedExchangeCallDoesNotHoldUpTheManager(t *testing.T) {
	st := NewStores(testdb.Open(t))
	stuckUser := testdb.AddUser(t, st.DB)
	otherUser := testdb.AddOtherUser(t, st.DB)
	stuckID := addLoadableTrader(t, st, stuckUser)
	otherID := addLoadableTrader(t, st, otherUser)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var stuckBuilds atomic.Int32

	tm := NewManager(st)
	tm.newTrader = func(config AutoTraderConfig, _ *Stores, _ string) (*AutoTrader, error) {
		if config.ID == stuckID {
			stuckBuilds.Add(1)
			entered <- struct{}{}
			<-release
		}
		return newIdleTrader(), nil
	}

	loaded := make(chan error, 1)
	go func() { loaded <- tm.EnsureUserTradersLoaded(stuckUser) }()
	awaitBuild(t, entered, loaded)

	finishes(t, "another user's load", func() {
		if err := tm.EnsureUserTradersLoaded(otherUser); err != nil {
			t.Errorf("load the other user: %v", err)
		}
	})
	if _, err := tm.GetTrader(otherID); err != nil {
		t.Errorf("the other user's trader should be loaded: %v", err)
	}
	finishes(t, "the read-only methods", func() {
		_, _ = tm.GetTrader(stuckID)
		_ = tm.GetLoadError(stuckID)
		_ = tm.GetAllTraders()
		_ = tm.GetTraderIDs()
	})
	finishes(t, "RemoveTrader", func() { tm.RemoveTrader(otherID) })
	finishes(t, "a polled read of the trader being loaded", func() {
		if err := tm.EnsureUserTradersLoaded(stuckUser); err != nil {
			t.Errorf("polled load: %v", err)
		}
	})

	releaseOnce.Do(func() { close(release) })
	if err := <-loaded; err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := tm.GetTrader(stuckID); err != nil {
		t.Errorf("the trader should be loaded once the exchange answers: %v", err)
	}
	if got := stuckBuilds.Load(); got != 1 {
		t.Errorf("trader built %d times, want the polled read to leave it to the load under way", got)
	}
}

func TestFirstLoadToFinishWins(t *testing.T) {
	st := NewStores(testdb.Open(t))
	userID := testdb.AddUser(t, st.DB)
	traderID := addLoadableTrader(t, st, userID)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	slow, fast := newIdleTrader(), newIdleTrader()
	var builds atomic.Int32

	tm := NewManager(st)
	tm.newTrader = func(AutoTraderConfig, *Stores, string) (*AutoTrader, error) {
		if builds.Add(1) == 1 {
			entered <- struct{}{}
			<-release
			return slow, nil
		}
		return fast, nil
	}

	loaded := make(chan error, 1)
	go func() { loaded <- tm.LoadUserTradersFromStore(userID) }()
	awaitBuild(t, entered, loaded)

	// An explicit load does not wait for one under way, so it gets there first.
	if err := tm.LoadUserTradersFromStore(userID); err != nil {
		t.Fatalf("second load: %v", err)
	}
	close(release)
	if err := <-loaded; err != nil {
		t.Fatalf("first load: %v", err)
	}

	got, err := tm.GetTrader(traderID)
	if err != nil {
		t.Fatalf("GetTrader: %v", err)
	}
	if got != fast {
		t.Error("the slower load replaced the trader that was already loaded")
	}
}

func TestRemovalDropsLoadAlreadyUnderWay(t *testing.T) {
	st := NewStores(testdb.Open(t))
	userID := testdb.AddUser(t, st.DB)
	traderID := addLoadableTrader(t, st, userID)

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var builds atomic.Int32

	tm := NewManager(st)
	tm.newTrader = func(AutoTraderConfig, *Stores, string) (*AutoTrader, error) {
		if builds.Add(1) == 1 {
			entered <- struct{}{}
			<-release
		}
		return newIdleTrader(), nil
	}

	loaded := make(chan error, 1)
	go func() { loaded <- tm.EnsureUserTradersLoaded(userID) }()
	awaitBuild(t, entered, loaded)

	// The trader's row changed and it was removed: what that load read is out of date,
	// and adding it would bring back the old config, and maybe start it.
	tm.RemoveTrader(traderID)
	close(release)
	if err := <-loaded; err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := tm.GetTrader(traderID); err == nil {
		t.Fatal("a load from before the removal added the trader back")
	}

	if err := tm.LoadUserTradersFromStore(userID); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := tm.GetTrader(traderID); err != nil {
		t.Errorf("a load from after the removal should add it: %v", err)
	}
}
