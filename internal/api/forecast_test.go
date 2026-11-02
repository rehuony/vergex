package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vergex/internal/database/testdb"
	"vergex/internal/forecast"
	"vergex/internal/market/feed"

	"github.com/gin-gonic/gin"
)

func forecastServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	marketStore := feed.NewStore(testdb.Open(t))
	store := forecast.NewStore(marketStore.DB())
	service := feed.NewService(marketStore)
	s := &Server{marketData: service, forecast: forecast.NewEngine(store, service, nil)}
	r := gin.New()
	r.GET("/strategies", s.handleForecastStrategies)
	r.POST("/strategies", s.handleCreateForecastStrategy)
	r.POST("/strategies/:id/activate", s.handleActivateForecastStrategy)
	r.PUT("/settings", s.handleUpdateForecastSettings)
	r.POST("/backtests", s.handleCreateForecastBacktest)
	r.GET("/signals", s.handleForecastCoins)
	return r
}

func send(r http.Handler, method, url, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, url, strings.NewReader(body)))
	return w
}

func TestForecastEndpointsAreUnavailableWithoutTheEngine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &Server{}
	r := gin.New()
	r.GET("/signals", s.handleForecastCoins)
	r.GET("/status", s.handleMarketDataStatus)
	for _, path := range []string{"/signals", "/status"} {
		if w := send(r, http.MethodGet, path, ""); w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s = %d", path, w.Code)
		}
	}
}

func TestForecastStrategiesAreValidated(t *testing.T) {
	r := forecastServer(t)
	if w := send(r, http.MethodGet, "/strategies", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active_strategy_id":"`) {
		t.Fatalf("strategies: %d %s", w.Code, w.Body.String())
	}
	bad := `{"config":{"scorer":"weighted","factors":{},"adx_threshold":25,"slope":3,"direction_threshold":0.05,"horizons":{"15m":4,"1h":4,"4h":4,"1d":4}}}`
	if w := send(r, http.MethodPost, "/strategies", bad); w.Code != http.StatusBadRequest {
		t.Fatalf("a config with no factors = %d", w.Code)
	}
	if w := send(r, http.MethodPost, "/strategies/99/activate", ""); w.Code != http.StatusNotFound {
		t.Fatalf("activating a missing strategy = %d", w.Code)
	}
}

func TestForecastInputsAreChecked(t *testing.T) {
	r := forecastServer(t)
	if w := send(r, http.MethodGet, "/signals?interval=3m", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("unsupported interval = %d", w.Code)
	}
	if w := send(r, http.MethodGet, "/signals?interval=1h", ""); w.Code != http.StatusOK {
		t.Fatalf("signals = %d %s", w.Code, w.Body.String())
	}
	if w := send(r, http.MethodPut, "/settings", `{"alert_threshold":40}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a threshold under 51 = %d", w.Code)
	}
	if w := send(r, http.MethodPut, "/settings", `{"alert_threshold":80}`); w.Code != http.StatusOK {
		t.Fatalf("threshold = %d %s", w.Code, w.Body.String())
	}
	if w := send(r, http.MethodPost, "/backtests", `{"strategy_id":1,"symbols":["bad symbol"],"intervals":["1h"],"from":1,"to":2}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad symbol = %d", w.Code)
	}
	if w := send(r, http.MethodPost, "/backtests", `{"strategy_id":1,"symbols":["BTCUSDT"],"intervals":["3m"],"from":1,"to":2}`); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad interval = %d", w.Code)
	}
}
