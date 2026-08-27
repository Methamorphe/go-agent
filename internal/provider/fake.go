package provider

import (
	"context"
	"sync"

	"github.com/Methamorphe/go-agent/internal/errs"
)

type FakeScript struct {
	Events []Event
	Result Result
	Err    error
}

type Fake struct {
	mu      sync.Mutex
	scripts []FakeScript
	calls   []Request
	next    int
}

func NewFake(scripts ...FakeScript) *Fake {
	copyScripts := append([]FakeScript(nil), scripts...)
	return &Fake{scripts: copyScripts}
}

func (f *Fake) Name() string { return "fake" }

func (f *Fake) Stream(ctx context.Context, req Request, sink EventSink) (Result, error) {
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	if sink == nil {
		return Result{}, errs.New(errs.CodeInvalidArgument, "provider.fake.stream", "sink is nil")
	}

	f.mu.Lock()
	if f.next >= len(f.scripts) {
		f.mu.Unlock()
		return Result{}, errs.New(errs.CodeNotFound, "provider.fake.stream", "no scripted invocation remains")
	}
	script := f.scripts[f.next]
	f.next++
	f.calls = append(f.calls, req)
	f.mu.Unlock()

	for _, event := range script.Events {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err := event.Validate(); err != nil {
			return Result{}, err
		}
		if err := sink(event); err != nil {
			return Result{}, err
		}
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if script.Err != nil {
		return Result{}, script.Err
	}

	if script.Result.Provider == "" {
		script.Result.Provider = f.Name()
	}
	if script.Result.Model == "" {
		script.Result.Model = req.Model
	}
	if script.Result.FinishReason == "" {
		script.Result.FinishReason = FinishStop
	}
	return script.Result, nil
}

func (f *Fake) Calls() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.calls...)
}
