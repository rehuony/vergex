package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vergex/internal/account"
	"vergex/internal/database/testdb"
	"vergex/internal/market"
)

func TestVerifyAccountChecksOneAccountPastTheCache(t *testing.T) {
	st := NewStores(testdb.Open(t))
	owner := testdb.AddUser(t, st.DB)
	id, err := st.Accounts.Create(owner, "binance", "Main", "fixture-key", "fixture-secret", "", false)
	if err != nil {
		t.Fatalf("create exchange: %v", err)
	}
	// An account with a credential missing is answered without calling the exchange.
	if err := st.DB.Model(&account.Account{}).Where("id = ?", id).Update("secret_key", "").Error; err != nil {
		t.Fatalf("blank the secret: %v", err)
	}

	s := &Server{stores: st, accountStateCache: NewAccountStateCache()}
	s.accountStateCache.Set(owner, map[string]AccountState{
		id: {AccountID: id, Status: accountStatusOK},
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", owner) })
	r.POST("/accounts/:id/verify", s.handleVerifyAccount)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts/"+id+"/verify", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("verify status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var state AccountState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.AccountID != id || state.Status != accountStatusMissingCredentials {
		t.Fatalf("state = %+v, want %s missing credentials", state, id)
	}
	cached, ok := s.accountStateCache.Get(owner)
	if !ok || cached[id].Status != accountStatusMissingCredentials {
		t.Fatalf("cached state = %+v, want the fresh check", cached[id])
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/accounts/missing/verify", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown account status = %d, want 404", w.Code)
	}
}

type markSource struct {
	market.LiveSource
	watched []string
}

func (m *markSource) WatchMark(_, symbol string) { m.watched = append(m.watched, symbol) }
func (m *markSource) Mark(venue, symbol string) (float64, bool) {
	if venue == "binance" && symbol == "BTCUSDT" {
		return 80000.5, true
	}
	return 0, false
}

func TestAccountMarksComeFromTheLiveStream(t *testing.T) {
	st := NewStores(testdb.Open(t))
	owner := testdb.AddUser(t, st.DB)
	id, err := st.Accounts.Create(owner, "binance", "Main", "fixture-key", "fixture-secret", "", false)
	if err != nil {
		t.Fatal(err)
	}
	source := &markSource{}
	market.SetLiveSource(source)
	t.Cleanup(func() { market.SetLiveSource(noMarks{}) })

	s := &Server{stores: st}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", owner) })
	r.GET("/accounts/:id/marks", s.handleAccountMarks)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/accounts/"+id+"/marks?symbols=BTCUSDT,ETHUSDT,not%20a%20symbol", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Marks map[string]float64 `json:"marks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Marks) != 1 || body.Marks["BTCUSDT"] != 80000.5 {
		t.Fatalf("marks: %+v", body.Marks)
	}
	if len(source.watched) != 2 {
		t.Fatalf("watched: %v", source.watched)
	}
}

type noMarks struct{ market.LiveSource }

func (noMarks) WatchMark(_, _ string)            {}
func (noMarks) Mark(_, _ string) (float64, bool) { return 0, false }

type deadKeyReader struct{ accountReader }

func (deadKeyReader) GetPositions() ([]map[string]interface{}, error) {
	return nil, errors.New("Invalid API-key, IP, or permissions for action")
}

func TestPositionsSayWhyTheyCannotBeRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	(&Server{}).writePositions(c, deadKeyReader{})
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d", w.Code)
	}
	var body struct {
		Error    string `json:"error"`
		ErrorKey string `json:"error_key"`
		Status   string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ErrorKey != "accountError.PERMISSION_DENIED" && body.ErrorKey != "accountError.INVALID_CREDENTIALS" {
		t.Errorf("body = %+v", body)
	}
}
