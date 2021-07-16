package workerctl

import (
	"fmt"
	"io"
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
