package telegram

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vergex/internal/config"
	"vergex/internal/store/storetest"
)

// fakeBotAPI answers getMe for tokens starting with "good" and reports every
// getMe call on the returned channel.
func fakeBotAPI(t *testing.T) <-chan string {
	t.Helper()
	calls := make(chan string, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Paths look like /bot<token>/<method>.
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/bot"), "/", 2)
		token, method := parts[0], parts[1]
		w.Header().Set("Content-Type", "application/json")
		switch {
		case method == "getMe":
			calls <- token
			if !strings.HasPrefix(token, "good") {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
				return
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":1,"is_bot":true,"first_name":"Vergex","username":"vergex_test_bot"}}`))
		case method == "getUpdates":
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
		default:
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		}
	}))
	t.Cleanup(srv.Close)

	origEndpoint, origDelay := apiEndpoint, retryDelay
	apiEndpoint = srv.URL + "/bot%s/%s"
	retryDelay = time.Hour // only a reload may restart a failed bot here
	t.Cleanup(func() { apiEndpoint, retryDelay = origEndpoint, origDelay })
	return calls
}

func expectGetMe(t *testing.T, calls <-chan string, want string) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("bot started with token %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("bot never started with token %q", want)
	}
}

func TestStartReloadsWithSavedTokenAndRecoversFromBadToken(t *testing.T) {
	calls := fakeBotAPI(t)
	st := storetest.New(t)
	t.Cleanup(func() { _ = st.Close() })

	reload := make(chan struct{}, 1)
	save := func(token string) {
		if err := st.TelegramConfig().Save(token, ""); err != nil {
			t.Fatalf("save token: %v", err)
		}
		reload <- struct{}{}
	}

	// No token yet: the bot waits for one.
	go Start(&config.Config{}, st, reload)
	select {
	case got := <-calls:
		t.Fatalf("bot started without a token (%q)", got)
	case <-time.After(100 * time.Millisecond):
	}

	save("good-1")
	expectGetMe(t, calls, "good-1")

	// A running bot restarts with a new token.
	save("bad-2")
	expectGetMe(t, calls, "bad-2")

	// A failed start does not end the supervisor: the next save is used.
	save("good-3")
	expectGetMe(t, calls, "good-3")
}
