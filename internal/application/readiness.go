package application

import (
	"context"
	"errors"
	"sync/atomic"
)

var errNotReady = errors.New("application is not ready")

type readiness struct {
	ready atomic.Bool
	ping  func(context.Context) error
}

func (r *readiness) Check(ctx context.Context) error {
	if !r.ready.Load() {
		return errNotReady
	}
	if r.ping != nil {
		return r.ping(ctx)
	}
	return nil
}

func (r *readiness) set(value bool) { r.ready.Store(value) }
