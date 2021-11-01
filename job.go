package workerctl

import (
	"context"
	"sync"
)

type JobRunner struct {
	wg           sync.WaitGroup
	PanicHandler func(interface{})
}

// Run :
func (r *JobRunner) Run(fn func()) {
	r.wg.Add(1)
	go func() {
		defer func() {
			r.wg.Done()
			if r.PanicHandler != nil {
				rvr := recover()
				if rvr != nil {
					r.PanicHandler(rvr)
				}
			}
		}()
		fn()
	}()
}

// Wait :
func (r *JobRunner) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.wg.Wait()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
