package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestFailuresAreToldApartByWhatToDoAboutThem(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want FailureKind
	}{
		{"bad key", &StatusError{Status: 401, Body: `{"error":{"message":"invalid"}}`}, FailureAuth},
		{"forbidden", &StatusError{Status: 403, Body: `nope`}, FailureAuth},
		{"payment", &StatusError{Status: 402, Body: `Insufficient Balance`}, FailureBalance},
		{"quota on 429", &StatusError{Status: 429, Body: `{"error":{"message":"You exceeded your current quota"}}`}, FailureBalance},
		{"rate limit", &StatusError{Status: 429, Body: `slow down`}, FailureRateLimit},
		{"no such path", &StatusError{Status: 404, Body: `not found`}, FailureModelNotFound},
		{"model on 400", &StatusError{Status: 400, Body: `{"error":{"message":"The supported API model names are a, b, but you passed c."}}`}, FailureModelNotFound},
		{"model on 400, other words", &StatusError{Status: 400, Body: `{"error":"model ` + "`x`" + ` does not exist"}`}, FailureModelNotFound},
		{"too long", &StatusError{Status: 400, Body: `This model's maximum context length is 8192 tokens`}, FailureContextLength},
		{"too large", &StatusError{Status: 413, Body: ``}, FailureContextLength},
		{"key on 400", &StatusError{Status: 400, Body: `API key not valid`}, FailureAuth},
		{"overloaded", &StatusError{Status: 529, Body: `overloaded_error`}, FailureOverloaded},
		{"server error", &StatusError{Status: 500, Body: ``}, FailureOverloaded},
		{"bad request", &StatusError{Status: 400, Body: `bad temperature`}, FailureOther},
		{"retried", fmt.Errorf("still failed after 3 retries: %w", &StatusError{Status: 503}), FailureOverloaded},
		{"out of time", fmt.Errorf("failed to send request: %w", context.DeadlineExceeded), FailureTimeout},
		{"no connection", errors.New("failed to send request: dial tcp: no such host"), FailureNetwork},
	}
	for _, c := range cases {
		if got := Classify(nil, c.err); got.Kind != c.want {
			t.Errorf("%s: %s, want %s", c.name, got.Kind, c.want)
		}
	}
}

func TestAFailureKeepsTheProvidersLineWithoutTheKey(t *testing.T) {
	client := NewClient(WithAPIKey("sk-secret"))
	failure := Classify(client, fmt.Errorf("still failed after 3 retries: %w", &StatusError{
		Status: 401, Body: `{"error":{"message":"Key sk-secret is not valid"}}`,
	}))
	if failure.Kind != FailureAuth || strings.Contains(failure.Error(), "sk-secret") {
		t.Fatalf("failure = %+v", failure)
	}
	if !strings.Contains(failure.Detail, "is not valid") || !strings.Contains(failure.Describe(true), "服务商拒绝") {
		t.Fatalf("detail = %q, zh = %q", failure.Detail, failure.Describe(true))
	}
	if failure.Key() != "agentError.auth" {
		t.Fatalf("key = %q", failure.Key())
	}
}

func TestAFailureWithNoAnswerHasNoDetail(t *testing.T) {
	failure := Classify(nil, errors.New(`Post "https://api.example.com/x": dial tcp: connection refused`))
	if failure.Detail != "" || strings.Contains(failure.Error(), "example.com") {
		t.Fatalf("failure = %+v", failure)
	}
}
