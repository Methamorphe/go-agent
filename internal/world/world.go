package world

import (
	"context"
	"errors"
	"time"
)

var ErrDenied = errors.New("world action denied")

type Executor interface {
	Profile() Profile
	Execute(context.Context, Action) (Result, error)
}

type SecureWorld struct {
	authorizer *Authorizer
	inner      Executor
	now        func() time.Time
}

func NewSecureWorld(authorizer *Authorizer, inner Executor, now func() time.Time) *SecureWorld {
	if now == nil {
		now = time.Now
	}
	return &SecureWorld{authorizer: authorizer, inner: inner, now: now}
}

func (w *SecureWorld) Profile() Profile { return w.inner.Profile() }

func (w *SecureWorld) Execute(ctx context.Context, action Action) (Result, error) {
	decision := w.authorizer.Authorize(action, w.now().UTC())
	if !decision.Allowed {
		return Result{Status: ResultDenied, Error: decision.Code}, ErrDenied
	}
	return w.inner.Execute(ctx, action)
}
