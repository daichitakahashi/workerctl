package workerctl

import (
	"context"
	"fmt"
	"io"
	"sync"

	"go.uber.org/multierr"
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
		err = multierr.Append(err, e)
	}
	return
}

func (c Closer) CloseOnError(err error) error {
	if err != nil {
		return multierr.Append(err, c.Close())
	}
	return nil
}

type transferCtx struct {
	context.Context
	values context.Context
}

// Transfer transfers holder's values to new context based on ctx.
// It enables us to access attached values of holder via a new context.
// Lifetime of new context is detached from lifetime of holder.
func Transfer(ctx, holder context.Context) context.Context {
	return &transferCtx{
		Context: ctx,
		values:  holder,
	}
}

func (c *transferCtx) Value(key interface{}) interface{} {
	v := c.values.Value(key)
	if v != nil {
		return v
	}
	return c.Context.Value(key)
}

// Transferred is shorthand of Transfer(context.Background(), holder).
func Transferred(holder context.Context) context.Context {
	return Transfer(context.Background(), holder)
}
