package market

import (
	"net/http"
	"testing"

	"vergex/internal/venue/binance"
)

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPublicReadsTellTheLimiterWithoutWaitingForIt(t *testing.T) {
	limiter := binance.NewLimiter()
	ObservePublicResponses(limiter.Observe)
	t.Cleanup(func() { publicObserver.Store(nil) })

	calls := 0
	transport := observedTransport{roundTripper(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Retry-After": {"60"}}, Body: http.NoBody}, nil
	})}
	client := &http.Client{Transport: transport}
	for i := 0; i < 2; i++ {
		response, err := client.Get("https://fapi.binance.example/fapi/v1/depth")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
	}
	if calls != 2 {
		t.Fatalf("a paused limiter held up a public read: %d calls", calls)
	}
	if limiter.Status().PausedUntil.IsZero() {
		t.Fatal("the limiter never heard of the 429")
	}
}
