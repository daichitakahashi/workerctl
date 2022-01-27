package workerctl

import (
	"context"
	"io"
	"sync"
)

type (
	Controller interface {
		// Dependent creates new Controller depends on parent.
		Dependent() Controller

		// Launch registers WorkerLauncher to this Controller and call it.
		// Return error when LaunchWorker cause error.
		Launch(l WorkerLauncher) error

		// Bind resource to Controller.
		// After completion of controller's shutdown, resources are to be closed.
		Bind(rc io.Closer)

		// Context returns Controller's Context.
		Context() context.Context

		// WithContext returns a Controller with its context changed to ctx.
		// The provided ctx must be non-nil.
		WithContext(ctx context.Context) Controller
	}

	ShutdownFunc func(ctx context.Context) error

	WorkerLauncher interface {
		LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error)
	}

	Func func(ctx context.Context) (stop func(ctx context.Context), err error)
)

func (f Func) LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error) {
	return f(ctx)
}

type root struct {
	*controller
	shutdownCtx  context.Context
	shutdownOnce sync.Once
	determined   chan struct{}
}

func New(ctx context.Context) (Controller, ShutdownFunc) {
	parentCtx, cancel := context.WithCancel(ctx)
	r := &root{
		determined: make(chan struct{}),
	}
	r.controller = &controller{
		root:         r,
		ctx:          parentCtx,
		dependentsWg: &sync.WaitGroup{},
		wg:           &sync.WaitGroup{},
		shutdown:     make(chan struct{}),
	}

	done := make(chan struct{})
	go PanicSafe(func() error {
		<-parentCtx.Done()
		<-r.wait()
		close(done)
		return nil
	})

	return r, func(ctx context.Context) error {
		// determine shutdown context.
		shutdownCtx := r.determineShutdownContext(ctx)

		// cancel main context.
		cancel()

		select {
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		case <-parentCtx.Done():
			<-done
		}
		return nil
	}
}

type key int8

const (
	defaultShutdownKey key = iota + 1
	abortKey
)

func WithDefaultShutdownContext(ctx context.Context, newShutdownCtx func(ctx context.Context) context.Context) context.Context {
	return context.WithValue(ctx, defaultShutdownKey, newShutdownCtx)
}

func WithAbort(ctx context.Context, a *Aborter) context.Context {
	return context.WithValue(ctx, abortKey, a)
}

func Abort(ctx context.Context) {
	v := ctx.Value(abortKey)
	if v != nil {
		v.(*Aborter).Abort()
	}
}

func (r *root) determineShutdownContext(ctx context.Context) context.Context {
	r.shutdownOnce.Do(func() {
		if ctx != nil {
			r.shutdownCtx = ctx
		} else {
			v := r.ctx.Value(defaultShutdownKey)
			if newShutdownCtx, ok := v.(func(context.Context) context.Context); ok {
				r.shutdownCtx = newShutdownCtx(context.Background())
			} else {
				r.shutdownCtx = context.Background()
			}
		}
		close(r.determined)
	})
	<-r.determined
	return r.shutdownCtx
}

var _ Controller = (*root)(nil)

type controller struct {
	root         *root
	ctx          context.Context
	dependentsWg *sync.WaitGroup
	wg           *sync.WaitGroup
	shutdown     chan struct{}
	rcs          Closer
	m            sync.Mutex
}

func (c *controller) wait() <-chan struct{} {
	done := make(chan struct{})
	go PanicSafe(func() error {
		defer close(done)
		c.dependentsWg.Wait()
		close(c.shutdown)
		c.wg.Wait()
		_ = c.rcs.Close()
		return nil
	})
	return done
}

func (c *controller) Launch(l WorkerLauncher) error {
	c.m.Lock()
	defer c.m.Unlock()

	// prevent launch after Shutdown.
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
	}

	stop, err := l.LaunchWorker(c.ctx)
	if err != nil {
		return err
	}

	c.wg.Add(1)
	go PanicSafe(func() error {
		defer c.wg.Done()
		select {
		case <-c.shutdown:
			ctx := c.root.determineShutdownContext(nil)
			stop(ctx)
		}
		return nil
	})
	return nil
}

func (c *controller) Dependent() Controller {
	dependent := &controller{
		root:         c.root,
		ctx:          c.ctx,
		dependentsWg: &sync.WaitGroup{},
		wg:           &sync.WaitGroup{},
		shutdown:     make(chan struct{}),
	}
	c.dependentsWg.Add(1)
	go PanicSafe(func() error {
		defer c.dependentsWg.Done()
		select {
		case <-c.ctx.Done():
			<-dependent.wait()
		}
		return nil
	})
	return dependent
}

func (c *controller) Bind(rc io.Closer) {
	c.m.Lock()
	defer c.m.Unlock()
	c.rcs = append(c.rcs, rc)
}

func (c *controller) Context() context.Context {
	return c.ctx
}

func (c *controller) WithContext(ctx context.Context) Controller {
	return &controller{
		root:         c.root,
		ctx:          ctx,
		dependentsWg: c.dependentsWg,
		wg:           c.wg,
		rcs:          c.rcs,
	}
}

var _ Controller = (*controller)(nil)
