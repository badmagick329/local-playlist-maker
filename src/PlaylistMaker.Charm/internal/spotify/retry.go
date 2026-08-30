package spotify

import (
	"context"
	"errors"
	"time"
)

// RetryRateLimit retries one request after a valid short Spotify Retry-After delay.
func RetryRateLimit[T any](ctx context.Context, wait func(context.Context, time.Duration) error, operation func() (T, error)) (T, error) {
	value, err := operation()
	var rateLimit *RateLimitError
	if err == nil || !errors.As(err, &rateLimit) || !rateLimit.Valid || rateLimit.RetryAfter > 30*time.Second {
		return value, err
	}
	if wait == nil {
		wait = waitContext
	}
	if err := wait(ctx, rateLimit.RetryAfter); err != nil {
		var zero T
		return zero, err
	}
	return operation()
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
