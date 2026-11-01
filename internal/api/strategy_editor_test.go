package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vergex/internal/store"
	"vergex/internal/store/storetest"

	"github.com/gin-gonic/gin"
)

const editedStrategy = "00000000-0000-4000-8000-0000000000a1"

func strategyServer(t *testing.T) (*store.Store, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	st := storetest.New(t)
	storetest.Account(t, st)
	s := &Server{store: st}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", storetest.UserID) })
	r.GET("/strategies/default-config", s.handleGetDefaultStrategyConfig)
	r.PUT("/strategies/:id", s.handleUpdateStrategy)
	return st, r
}

func TestDefaultConfigByStrategyType(t *testing.T) {
	_, r := strategyServer(t)

	var grid map[string]any
	if code := getJSON(t, r, "/strategies/default-config?type=grid_trading", &grid); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if grid["strategy_type"] != "grid_trading" || grid["grid_config"] == nil || grid["agent_config"] != nil {
		t.Fatalf("expected a grid-only config, got %v", grid)
	}

	var agentConfig map[string]any
	if code := getJSON(t, r, "/strategies/default-config", &agentConfig); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if agentConfig["agent_config"] == nil || agentConfig["grid_config"] != nil {
		t.Fatalf("expected an agent-only config, got %v", agentConfig)
	}
}

func TestUpdateStrategyDescriptionAndGridLimits(t *testing.T) {
	st, r := strategyServer(t)
	grid := store.DefaultGridStrategyConfig()
	config, err := json.Marshal(store.StrategyConfig{StrategyType: "grid_trading", GridConfig: &grid})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Strategy().Create(&store.Strategy{ID: editedStrategy, UserID: storetest.UserID, Name: "Range", Description: "Old note", Config: string(config)}); err != nil {
		t.Fatal(err)
	}

	put := func(body string) {
		t.Helper()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/strategies/"+editedStrategy, bytes.NewBufferString(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
	}

	// Leaving the description out keeps it, so other callers cannot wipe it.
	put(`{"name":"Range"}`)
	saved, err := st.Strategy().Get(storetest.UserID, editedStrategy)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Description != "Old note" {
		t.Fatalf("description changed to %q", saved.Description)
	}

	// An empty description clears it, and grid limits are pulled into range.
	put(`{"description":"","config":{"grid_config":{"grid_count":500,"leverage":99}}}`)
	saved, err = st.Strategy().Get(storetest.UserID, editedStrategy)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Description != "" {
		t.Fatalf("description not cleared: %q", saved.Description)
	}
	var stored store.StrategyConfig
	if err := json.Unmarshal([]byte(saved.Config), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.GridConfig.GridCount != store.MaxGridCount || stored.GridConfig.Leverage != store.MaxGridLeverage {
		t.Fatalf("grid limits not applied: %+v", stored.GridConfig)
	}
}
