package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// statusClient answers every call with a status and body, and counts them.
func statusClient(status int, body string, header http.Header, retries int, wait time.Duration) (*BaseClient, *int) {
	mockHTTP := NewMockHTTPClient()
	calls := 0
	mockHTTP.ResponseFunc = func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	}
	client := NewClient(WithHTTPClient(mockHTTP.ToHTTPClient()), WithLogger(NewMockLogger()), WithAPIKey("key"),
		WithMaxRetries(retries), WithRetryWaitBase(wait)).(*BaseClient)
	return client, &calls
}

func TestStatusDecidesWhatIsTriedAgain(t *testing.T) {
	cases := []struct {
		status int
		body   string
		calls  int
	}{
		{http.StatusTooManyRequests, `{"error":"slow down"}`, 3},
		{http.StatusInternalServerError, `oops`, 3},
		{http.StatusBadGateway, `oops`, 3},
		{http.StatusServiceUnavailable, `oops`, 3},
		{http.StatusGatewayTimeout, `oops`, 3},
		{529, `{"type":"overloaded_error"}`, 3},
		// These fail the same way however long they wait, whatever the body says.
		{http.StatusBadRequest, `{"error":"unexpected EOF in your prompt, timeout"}`, 1},
		{http.StatusUnauthorized, `{"error":"bad key"}`, 1},
		{http.StatusPaymentRequired, `{"error":"no balance"}`, 1},
		{http.StatusNotFound, `{"error":"no such model"}`, 1},
	}
	for _, c := range cases {
		client, calls := statusClient(c.status, c.body, http.Header{}, 3, time.Millisecond)
		_, err := client.CallWithMessages("system", "user")
		if err == nil {
			t.Fatalf("status %d: expected an error", c.status)
		}
		if *calls != c.calls {
			t.Errorf("status %d: %d calls, want %d", c.status, *calls, c.calls)
		}
		var refused *StatusError
		if !errors.As(err, &refused) || refused.Status != c.status {
			t.Errorf("status %d: the error lost its status: %v", c.status, err)
		}
	}
}

func TestRetryAfterIsWaitedFor(t *testing.T) {
	header := http.Header{"Retry-After": {"1"}}
	client, calls := statusClient(http.StatusTooManyRequests, `{}`, header, 2, time.Millisecond)
	started := time.Now()
	if _, err := client.CallWithMessages("system", "user"); err == nil {
		t.Fatal("expected an error")
	}
	if *calls != 2 {
		t.Fatalf("%d calls", *calls)
	}
	if elapsed := time.Since(started); elapsed < time.Second {
		t.Fatalf("waited only %v of the second asked for", elapsed)
	}
}

func TestRetryAfterIsReadWithinLimits(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"nonsense":                      0,
		"-5":                            0,
		"7":                             7 * time.Second,
		"3600":                          maxRetryAfter,
		"Mon, 02 Nov 2026 12:00:10 GMT": 10 * time.Second,
		"Mon, 02 Nov 2026 11:00:00 GMT": 0,
	}
	for header, want := range cases {
		if got := parseRetryAfter(header, now); got != want {
			t.Errorf("%q: %v, want %v", header, got, want)
		}
	}
}

func TestWaitingToRetryStopsWithTheContext(t *testing.T) {
	client, calls := statusClient(http.StatusTooManyRequests, `{}`, http.Header{"Retry-After": {"30"}}, 3, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	started := time.Now()
	_, err := client.CallWithRequestFull(&Request{Ctx: ctx, Model: "m", Messages: []Message{NewUserMessage("hi")}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if time.Since(started) > 5*time.Second || *calls != 1 {
		t.Fatalf("took %v and %d calls", time.Since(started), *calls)
	}
}

func TestAStoppedOrExpiredCallIsNotTriedAgain(t *testing.T) {
	client, _ := statusClient(http.StatusOK, `{}`, http.Header{}, 3, time.Millisecond)
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, fmt.Errorf("failed to send request: %w", context.Canceled)} {
		if client.IsRetryableError(err) {
			t.Errorf("%v should not be retried", err)
		}
	}
	if !client.IsRetryableError(errors.New("read tcp: connection reset by peer")) {
		t.Error("a dropped connection should be retried")
	}
}
