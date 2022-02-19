package workerctl

import (
	"context"
	"sync"
	"sync/atomic"
	"unsafe"
)

// JobRunner is one shot job executor.
// Execute job using Run or Go, and Wait for all running jobs are finished.
// This enables us to perform shutdown of background job gracefully, which executed outside request-response scope.
type JobRunner struct {
	wg      sync.WaitGroup
	waiting unsafe.Pointer

	// If PanicHandler is set, panic will be recovered and passed as v.
	// If not set, JobRunner doesn't recover.
	PanicHandler func(v interface{})
}

// Run executes job.
func (r *JobRunner) Run(ctx context.Context, fn func(context.Context)) {
	r.wg.Add(1)
	r.run(ctx, fn)
}

// Go spawns goroutine and executes job.
func (r *JobRunner) Go(ctx context.Context, fn func(context.Context)) {
	r.wg.Add(1)
	go r.run(ctx, fn)
}

func (r *JobRunner) run(ctx context.Context, fn func(ctx context.Context)) {
	defer func() {
		r.wg.Done()
		// if PanicHandler is not nil, recover panic and pass recovered value
		if r.PanicHandler != nil {
			rvr := recover()
			if rvr != nil {
				r.PanicHandler(rvr)
			}
		}
	}()
	fn(ctx)
}

// Wait for all running job finished.
// If ctx is cancelled or timed out, waiting is also cancelled.
func (r *JobRunner) Wait(ctx context.Context) error {
	waiting := make(chan struct{})
	if atomic.CompareAndSwapPointer(&r.waiting, unsafe.Pointer(nil), unsafe.Pointer(&waiting)) {
		go func() {
			defer close(waiting)
			r.wg.Wait()
			atomic.StorePointer(&r.waiting, unsafe.Pointer(nil))
		}()
	} else {
		waiting = *(*chan struct{})(atomic.LoadPointer(&r.waiting))
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-waiting:
		return nil
	}
}
