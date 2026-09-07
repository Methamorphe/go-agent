package scheduler

import (
	"context"
	"errors"
	"time"
)

type hedgeValue[T any] struct {
	value T
	err   error
	which int
}

// Hedge starts the secondary only after delay. The first successful result wins;
// cancellation is propagated to the loser. It is intentionally generic and has
// no side-effect semantics: callers must only use it for isolated/idempotent work.
func Hedge[T any](ctx context.Context, delay time.Duration, primary, secondary func(context.Context) (T, error)) (T, error) {
	var zero T
	if primary == nil || secondary == nil {
		return zero, errors.New("both hedge functions are required")
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make(chan hedgeValue[T], 2)
	run := func(which int, fn func(context.Context) (T, error)) {
		value, err := fn(ctx)
		select {
		case results <- hedgeValue[T]{value: value, err: err, which: which}:
		case <-ctx.Done():
		}
	}
	go run(1, primary)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	secondaryStarted := false
	var firstErr error
	for {
		select {
		case <-ctx.Done():
			if firstErr != nil {
				return zero, firstErr
			}
			return zero, ctx.Err()
		case <-timer.C:
			if !secondaryStarted {
				secondaryStarted = true
				go run(2, secondary)
			}
		case result := <-results:
			if result.err == nil {
				cancel()
				return result.value, nil
			}
			if firstErr == nil {
				firstErr = result.err
			}
			if result.which == 1 && !secondaryStarted {
				secondaryStarted = true
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				go run(2, secondary)
				continue
			}
			if secondaryStarted {
				select {
				case other := <-results:
					if other.err == nil {
						cancel()
						return other.value, nil
					}
					return zero, errors.Join(firstErr, other.err)
				case <-ctx.Done():
					return zero, firstErr
				}
			}
		}
	}
}
