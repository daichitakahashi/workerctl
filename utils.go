package workerctl

import (
	"fmt"
	"io"
	"sync/atomic"
)

// Go :
func Go(fn func(), recovered func(v interface{})) {
	go func() {
		defer func() {
			rvr := recover()
			if rvr != nil && recovered != nil {
				recovered(rvr)
			}
		}()
		fn()
	}()
}

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

type Abort struct {
	aborted chan struct{}
	state   int32
}

func NewAbort() *Abort {
	return &Abort{
		aborted: make(chan struct{}),
	}
}

func (a *Abort) Abort() {
	if atomic.SwapInt32(&a.state, 1) == 0 {
		close(a.aborted)
	}
}

func (a *Abort) AbortOnError(err error) {
	if err != nil {
		a.Abort()
	}
}

func (a *Abort) Aborted() <-chan struct{} {
	return a.aborted
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

type CloseFunc func() error

func (c CloseFunc) Close() error {
	return c()
}
