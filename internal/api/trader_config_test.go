package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/trader/kind"
)

func storedAgentConfig(t *testing.T, st *Stores, id string) *kind.AgentConfig {
	t.Helper()
	saved, err := st.Traders.Get(testdb.UserID, id)
	if err != nil {
		t.Fatal(err)
	}
	config, err := kind.Decode(saved.Kind, saved.Config)
	if err != nil {
		t.Fatal(err)
	}
	return config.(*kind.AgentConfig)
}

func TestUpdatingAConfigMergesItBySection(t *testing.T) {
	st := NewStores(testdb.Open(t))
	trader := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine"})
	r := traderRoutes(t, st)

	body := `{"config":{"risk_control":{"max_positions":99},"coin_source":{"static_coins":["dogeusdt"]}}}`
	if code := sendTo(r, http.MethodPut, "/traders/"+trader.ID, body); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	config := storedAgentConfig(t, st, trader.ID)
	if config.RiskControl.MaxPositions != kind.MaxPositions {
		t.Errorf("max positions not brought within bounds: %d", config.RiskControl.MaxPositions)
	}
	if want := kind.DefaultAgent().RiskControl.MinConfidence; config.RiskControl.MinConfidence != want {
		t.Errorf("a field left out changed: %d", config.RiskControl.MinConfidence)
	}
	if got := strings.Join(config.CoinSource.StaticCoins, ","); got != "DOGEUSDT" {
		t.Errorf("static coins = %s", got)
	}
	if saved, _ := st.Traders.Get(testdb.UserID, trader.ID); saved.Name != "Mine" {
		t.Errorf("a name left out changed: %s", saved.Name)
	}
}

func TestAConfigWithAWrongChoiceIsRefused(t *testing.T) {
	st := NewStores(testdb.Open(t))
	trader := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine"})
	r := traderRoutes(t, st)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/traders/"+trader.ID, strings.NewReader(`{"config":{"coin_source":{"source_type":"OI Top"}}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "source_type") {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if config := storedAgentConfig(t, st, trader.ID); config.CoinSource.SourceType != "static" {
		t.Fatalf("the refused config was saved: %+v", config.CoinSource)
	}
}

func TestDuplicatingATraderCopiesItsConfigNotItsState(t *testing.T) {
	st := NewStores(testdb.Open(t))
	source := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine", Running: true})
	r := traderRoutes(t, st)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/traders/"+source.ID+"/duplicate", strings.NewReader(`{"name":"Copy"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		TraderID string `json:"trader_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	copied, err := st.Traders.Get(testdb.UserID, created.TraderID)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Copy" || copied.Kind != source.Kind || copied.Running || copied.InitialBalance != 0 {
		t.Fatalf("copied = %+v", copied)
	}
	var want, got map[string]any
	_ = json.Unmarshal(source.Config, &want)
	_ = json.Unmarshal(copied.Config, &got)
	if len(got) == 0 || len(got) != len(want) {
		t.Fatalf("config = %s", copied.Config)
	}
}

func TestPresetsListEachKindDefaultFirst(t *testing.T) {
	st := NewStores(testdb.Open(t))
	r := traderRoutes(t, st)

	for query, want := range map[string]string{"": "balanced", "?kind=agent": "balanced", "?kind=grid": "default"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/traders/presets"+query, nil))
		var out struct {
			Presets []struct {
				ID string `json:"id"`
			} `json:"presets"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil || len(out.Presets) == 0 || out.Presets[0].ID != want {
			t.Errorf("%q: %s", query, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/traders/presets?kind=dca", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("unknown kind: status %d", w.Code)
	}
}

func TestDryRunRefusesAConfigThatCannotRun(t *testing.T) {
	st := NewStores(testdb.Open(t))
	r := traderRoutes(t, st)

	for _, body := range []string{`{"kind":"dca","config":{}}`, `{"kind":"grid","config":{"symbol":"BTCUSDT","distribution":"curvy"}}`} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/traders/dry-run", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d", body, w.Code)
		}
	}
}
