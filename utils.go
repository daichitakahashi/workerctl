package workerctl

import (
	"fmt"
	"io"
	"sync"

	"go.uber.org/multierr"
)

// RecoveredError represents fatal error that embraces recovered value.
type RecoveredError struct {
	Recovered interface{}
}

func (e *RecoveredError) Error() string {
	return fmt.Sprintf("recovered: %v", e.Recovered)
}

// PanicSafe calls fn and captures panic occurred in fn.
// When panic is captured, it returns RecoveredError embracing recovered value.
// If fn returns error, PanicSafe also returns it.
func PanicSafe(fn func() error) (err error) {
	defer func() {
		rvr := recover()
		if rvr != nil {
			err = &RecoveredError{rvr}
		}
	}()
	err = fn()
	return
}

// Aborter propagates abort of application.
// Intended for hooking shutdown from any worker.
// This is only state holder and propagator, thus actual shutdown must be caused by user.
type Aborter struct {
	ch     chan struct{}
	mu     sync.Mutex
	closed bool
	err    error
}

func (a *Aborter) aborted() chan struct{} {
	if a.ch == nil {
		a.ch = make(chan struct{})
	}
	return a.ch
}

// Aborted returns a channel that's closed when Aborter.Abort is called.
func (a *Aborter) Aborted() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.aborted()
}

func (a *Aborter) abort(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed {
		close(a.aborted())
		a.closed = true
		a.err = err
	}
}

// Abort tells an application to shut down.
// This may be called by multiple goroutines simultaneously.
// After the first call, subsequent calls do nothing.
func (a *Aborter) Abort() {
	a.abort(nil)
}

// AbortOnError tells an application to shut down with cause.
// Cause error can be got from Err.
// If err==nil, AbortOnError does nothing.
func (a *Aborter) AbortOnError(err error) {
	if err != nil {
		a.abort(err)
	}
}

// Err returns error set in AbortOnError.
func (a *Aborter) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

// Closer is a list of io.Closer.
// By use this, we can handle error on init functions that contain operations opening resource.
type Closer []io.Closer

// Close closes all appended resources in a reverse order.
func (c Closer) Close() (err error) {
	last := len(c) - 1
	for i := range c {
		e := c[last-i].Close()
		err = multierr.Append(err, e)
	}
	return
}

// CloseOnError closes all appended resources in a reverse order, only when err!=nil.
func (c Closer) CloseOnError(err error) error {
	if err != nil {
		return multierr.Append(err, c.Close())
	}
	return nil
}
