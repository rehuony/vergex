package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func TestCreateDefaultStrategiesUsesOneReadyToRunPreset(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st}
	userID := storetest.Account(t, st)
	if err := s.createDefaultStrategies(userID, "zh"); err != nil {
		t.Fatalf("createDefaultStrategies failed: %v", err)
	}

	strategies, err := st.Strategy().List(userID)
	if err != nil {
		t.Fatalf("List strategies failed: %v", err)
	}
	if len(strategies) != 1 {
		t.Fatalf("expected 1 default strategy, got %d", len(strategies))
	}

	byName := map[string]*store.Strategy{}
	activeCount := 0
	for _, strategy := range strategies {
		byName[strategy.Name] = strategy
		if strategy.IsActive {
			activeCount++
		}
		if strategy.Name == "Balanced Strategy" || strategy.Name == "Steady Strategy" || strategy.Name == "Aggressive Strategy" {
			t.Fatalf("legacy crypto-style default strategy still present: %s", strategy.Name)
		}
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly one active strategy, got %d", activeCount)
	}

	defaultStrategy := byName["默认加密货币策略"]
	if defaultStrategy == nil || !defaultStrategy.IsActive {
		t.Fatalf("default crypto strategy should exist and be active, got %+v", byName)
	}
	cfg, err := defaultStrategy.ParseConfig()
	if err != nil {
		t.Fatalf("default ParseConfig failed: %v", err)
	}
	if cfg.CoinSource.SourceType != "static" || len(cfg.CoinSource.StaticCoins) == 0 {
		t.Fatalf("default strategy should use a static crypto coin pool, got %+v", cfg.CoinSource)
	}
}

func TestCreateDefaultStrategiesSkipsAccountWithStrategies(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	userID := storetest.Account(t, st)
	customCfg := store.GetDefaultStrategyConfig("zh")
	custom := &store.Strategy{
		ID:          uuid.New().String(),
		UserID:      userID,
		Name:        "aa",
		Description: "user custom active strategy",
		IsActive:    true,
	}
	if err := custom.SetConfig(&customCfg); err != nil {
		t.Fatalf("custom SetConfig failed: %v", err)
	}
	if err := st.Strategy().Create(custom); err != nil {
		t.Fatalf("create custom failed: %v", err)
	}

	s := &Server{store: st}
	for _, lang := range []string{"zh", "en"} {
		if err := s.createDefaultStrategies(userID, lang); err != nil {
			t.Fatalf("createDefaultStrategies(%q) failed: %v", lang, err)
		}
	}

	strategies, err := st.Strategy().List(userID)
	if err != nil {
		t.Fatalf("List strategies failed: %v", err)
	}
	if len(strategies) != 1 || strategies[0].Name != "aa" || !strategies[0].IsActive {
		t.Fatalf("an account with strategies must not be seeded, got %+v", strategies)
	}
}

// Registration seeds the preset in the request language; listing afterwards
// (which sends no language) used to seed a second, Chinese copy.
func TestListStrategiesAfterRegistrationKeepsOnePreset(t *testing.T) {
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	s := &Server{store: st}
	userID := storetest.Account(t, st)
	if err := s.createDefaultStrategies(userID, "en"); err != nil {
		t.Fatalf("createDefaultStrategies failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("user_id", userID) })
	router.GET("/strategies", s.handleGetStrategies)
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/strategies", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("list strategies got %d: %s", w.Code, w.Body.String())
		}
	}

	strategies, err := st.Strategy().List(userID)
	if err != nil {
		t.Fatalf("List strategies failed: %v", err)
	}
	if len(strategies) != 1 || strategies[0].Name != "Default Crypto Strategy" {
		names := make([]string, 0, len(strategies))
		for _, strategy := range strategies {
			names = append(names, strategy.Name)
		}
		t.Fatalf("expected only the English preset, got %v", names)
	}
}
