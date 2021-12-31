package workerctl

import (
	"fmt"
	"io"
	"sync"
)

type RecoveredError struct {
	Recovered interface{}
}

func (e *RecoveredError) Error() string {
	return fmt.Sprintf("recovered: %v", e.Recovered)
}

// PanicSafe :
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

// Aborter :
type Aborter struct {
	ch     chan struct{}
	mu     sync.Mutex
	closed bool
}

func (a *Aborter) aborted() chan struct{} {
	if a.ch == nil {
		a.ch = make(chan struct{})
	}
	return a.ch
}

func (a *Aborter) Aborted() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.aborted()
}

func (a *Aborter) Abort() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.closed {
		close(a.aborted())
	}
}

func (a *Aborter) AbortOnError(err error) {
	if err != nil {
		a.Abort()
	}
}

// Closer :
type Closer []io.Closer

// Close :
func (c Closer) Close() (err error) {
	last := len(c) - 1
	for i := range c {
		e := c[last-i].Close()
		if e != nil && err == nil {
			err = e
		}
	}
	return
}
