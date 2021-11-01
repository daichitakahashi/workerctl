package workerctl

import (
	"context"
	"io"
	"sync"
)

type (
	Controller interface {
		Dependent() Controller

		// NewWorker register Worker to Controller and call Worker.Init.
		// Return error when Init cause error.
		NewWorker(worker Worker) error

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

	Worker interface {
		Init(ctx context.Context) error
		Shutdown(ctx context.Context)
	}
)

type root struct {
	*controller
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	shutdownOnce   sync.Once
	determined     chan struct{}
}

func New(ctx context.Context) (Controller, ShutdownFunc) {
	ctx, cancel := context.WithCancel(ctx)
	r := &root{
		determined: make(chan struct{}),
	}
	r.controller = &controller{
		root:         r,
		ctx:          ctx,
		dependentsWg: &sync.WaitGroup{},
		wg:           &sync.WaitGroup{},
	}

	return r, func(ctx context.Context) error {
		// determine shutdown context.
		shutdownCtx, shutdownCancel := r.determineShutdownContext(ctx)
		if shutdownCancel != nil {
			defer shutdownCancel()
		}

		// cancel main context.
		cancel()

		var err error
		select {
		case <-shutdownCtx.Done():
			err = shutdownCtx.Err()
		case <-r.wait():
		}
		return err
	}
}

type key int8

func WithDefaultShutdownContext(ctx context.Context, newShutdownCtx func() (context.Context, context.CancelFunc)) context.Context {
	return context.WithValue(ctx, key(1), newShutdownCtx)
}

func (r *root) determineShutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	r.shutdownOnce.Do(func() {
		if ctx != nil {
			r.shutdownCtx = ctx
		} else {
			v := r.ctx.Value(key(1))
			if newShutdownCtx, ok := v.(func() (context.Context, context.CancelFunc)); ok {
				r.shutdownCtx, r.shutdownCancel = newShutdownCtx()
			} else {
				r.shutdownCtx = context.Background()
			}
		}
		close(r.determined)
	})
	<-r.determined
	return r.shutdownCtx, r.shutdownCancel
}

var _ Controller = (*root)(nil)

type controller struct {
	root         *root
	ctx          context.Context
	dependentsWg *sync.WaitGroup
	wg           *sync.WaitGroup
	rcs          Closer
	m            sync.Mutex
}

func (c *controller) wait() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.dependentsWg.Wait()
		c.wg.Wait()
		_ = c.rcs.Close()
	}()
	return done
}

func (c *controller) NewWorker(worker Worker) error {
	c.m.Lock()
	defer c.m.Unlock()

	// prevent launch after Shutdown.
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
	}

	err := worker.Init(c.ctx)
	if err != nil {
		return err
	}

	c.wg.Add(1)
	Go(func() {
		defer c.wg.Done()
		select {
		case <-c.ctx.Done():
			ctx, _ := c.root.determineShutdownContext(nil)
			worker.Shutdown(ctx)
		}
	}, nil)
	return nil
}

func (c *controller) Dependent() Controller {
	ctl := &controller{
		root:         c.root,
		ctx:          c.ctx,
		dependentsWg: &sync.WaitGroup{},
		wg:           &sync.WaitGroup{},
	}
	ctl.dependentsWg.Add(1)
	Go(func() {
		defer c.dependentsWg.Done()
		select {
		case <-c.ctx.Done():
			<-ctl.wait()
		}
	}, nil)
	return ctl
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
