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

func (a *Abort) Abort(err error) {
	if err != nil {
		if atomic.SwapInt32(&a.state, 1) == 0 {
			close(a.aborted)
		}
	}
}

func (a *Abort) Aborted() <-chan struct{} {
	return a.aborted
}

// Closer :
type Closer []io.Closer

// Append :
func (c *Closer) Append(cl io.Closer) {
	*c = append(*c, cl)
}

func (c Closer) close() (err error) {
	last := len(c) - 1
	for i := range c {
		e := c[last-i].Close()
		if e != nil && err == nil {
			err = e
		}
	}
	return
}

// Close :
func (c Closer) Close(err error) error {
	e := c.close()
	if e != nil && err == nil {
		err = e
	}
	return err
}
