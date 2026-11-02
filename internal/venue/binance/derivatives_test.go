package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestOpenInterest(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		want   float64
		bad    bool
	}{
		{"known positive", 200, `{"openInterest":"123.45"}`, 123.45, false},
		{"measured zero", 200, `{"openInterest":"0"}`, 0, false},
		{"rate limited", 429, `{"code":-1003,"msg":"rate limit"}`, 0, true},
		{"server error with numeric body", 503, `{"openInterest":"123.45"}`, 0, true},
		{"missing", 200, `{}`, 0, true},
		{"API error", 200, `{"code":-1121,"msg":"Invalid symbol"}`, 0, true},
		{"empty", 200, `{"openInterest":""}`, 0, true},
		{"invalid number", 200, `{"openInterest":"unknown"}`, 0, true},
		{"negative", 200, `{"openInterest":"-1"}`, 0, true},
		{"nan", 200, `{"openInterest":"NaN"}`, 0, true},
		{"infinity", 200, `{"openInterest":"+Inf"}`, 0, true},
		{"overflow", 200, `{"openInterest":"1e999"}`, 0, true},
		{"wrong JSON type", 200, `{"openInterest":123}`, 0, true},
		{"malformed JSON", 200, `{`, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			client := NewDataClient(NewLimiter()).WithBaseURL(server.URL)
			got, err := client.LatestOpenInterest(context.Background(), "BTCUSDT")
			if tt.bad != (err != nil) {
				t.Fatalf("err = %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
