package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// maxRetryAfter bounds how long a call waits because a provider asked it to. A turn has a
// person waiting on it, and a longer wait is better reported than sat through.
const maxRetryAfter = 30 * time.Second

// StatusError is a model API answering with anything but 200.
type StatusError struct {
	Status     int
	RetryAfter time.Duration
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("API returned error (status %d): %s", e.Status, e.Body)
}

// Retryable says whether the same request may work later. Anything else, such as a bad key
// or an unknown model, will fail the same way however long it waits.
func (e *StatusError) Retryable() bool {
	switch e.Status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout,
		520, 522, 524, // Cloudflare in front of a provider.
		529: // Anthropic overloaded.
		return true
	}
	return false
}

// newStatusError reads a response the API refused.
func newStatusError(resp *http.Response, body []byte) *StatusError {
	return &StatusError{Status: resp.StatusCode, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()), Body: string(body)}
}

// parseRetryAfter reads a Retry-After header, which is seconds or a date, within what a call
// is willing to wait.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	wait := time.Duration(0)
	if seconds, err := strconv.Atoi(value); err == nil {
		wait = time.Duration(seconds) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		wait = at.Sub(now)
	}
	return max(0, min(wait, maxRetryAfter))
}

// retrying makes a call, and makes it again while its failure may pass. ctx ending stops the
// waiting between tries.
func retrying[T any](client *BaseClient, ctx context.Context, call func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	maxRetries := client.Cfg.MaxRetries
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			client.Log.Warnf("⚠️  model API call failed, retrying (%d/%d)...", attempt, maxRetries)
		}
		result, err := call()
		if err == nil {
			if attempt > 1 {
				client.Log.Infof("✓ model API retry succeeded")
			}
			return result, nil
		}
		lastErr = err
		if !client.Hooks.IsRetryableError(err) {
			return zero, err
		}
		if attempt < maxRetries {
			wait := client.Cfg.RetryWaitBase * time.Duration(attempt)
			var refused *StatusError
			if errors.As(err, &refused) && refused.RetryAfter > wait {
				wait = refused.RetryAfter
			}
			client.Log.Infof("⏳ Waiting %v before retry...", wait)
			if err := sleepWithContext(ctx, wait); err != nil {
				return zero, err
			}
		}
	}
	return zero, fmt.Errorf("still failed after %d retries: %w", maxRetries, lastErr)
}
