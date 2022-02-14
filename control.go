// Package workerctl controls initialization and shutdown of workers that consists of application.
// It aims to describe dependencies of them, and shutdown them in right order.
package workerctl

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/daichitakahashi/oncewait"
)

type (
	// Controller is core interface of workerctl package.
	// Controller can launch workers and also create dependent Controller. It enables to describe dependencies between workers.
	// In shutdown, the derived Controller shuts down first, and then the parent Controller starts shutting down.
	// The launched workers are associated with the Controller, and start shutdown when the associated Controller is in the shutdown phase.
	// By binding resources that implement io.Closer, we can close them at the end of the Controller shutdown.
	Controller interface {
		// Dependent creates new Controller depends on parent.
		// After all derived Controller's shutdown completed, parent Controller's shutdown will start.
		Dependent() Controller

		// Launch registers WorkerLauncher to this Controller and call it.
		// Return error when LaunchWorker cause error.
		// Failure of Launch makes no side effect on Controller's state.
		Launch(l WorkerLauncher) error

		// Bind resource to Controller.
		// After completion of controller's shutdown, resources will be closed.
		Bind(rc io.Closer)

		// Context returns Controller's Context.
		Context() context.Context

		// WithContext returns a Controller with its context changed to ctx.
		// The provided ctx must be non-nil.
		// It's assumed to set values to context with context.WithValue.
		WithContext(ctx context.Context) Controller
	}

	// ShutdownFunc is a return value of New. It shuts down created Controller.
	// Parameter ctx will be passed to shutdown functions of each worker, so we can set timeout using context.WithDeadline or context.WithTimeout.
	// It's assumed tobe used in combination with configuration of container shutdown.
	// For example, `docker stop --timeout` or task definition parameter `stopTimeout` on AWS ECS.
	ShutdownFunc func(ctx context.Context) error

	// WorkerLauncher is responsible for initializing the worker and returning its shutdown function.
	// If the worker fails to launch, it is expected to release all the resources that were initialized during the startup and then return error.
	WorkerLauncher interface {
		LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error)
	}

	// Func is an easy way to define WorkerLauncher.
	// Func(f) is a WorkerLauncher that calls f when passed to Controller.Launch.
	Func func(ctx context.Context) (stop func(ctx context.Context), err error)
)

// LaunchWorker calls f(ctx).
func (f Func) LaunchWorker(ctx context.Context) (stop func(ctx context.Context), err error) {
	return f(ctx)
}

// A root Controller.
type root struct {
	*controller
	shutdownCtx    context.Context
	cancelShutdown context.CancelFunc
	shutdownOnce   *oncewait.OnceWaiter
}

// New returns a new Controller which scope is bound to ctx and ShutdownFunc.
// When ctx canceled, Controller starts shutdown. Also calling ShutdownFunc does the same.
// However, cancellation of context.Context cannot set Context of shutdown.
// If you need, use WithDefaultShutdownContext to ctx before New.
func New(ctx context.Context) (Controller, ShutdownFunc) {
	parentCtx, cancel := context.WithCancel(ctx)
	r := &root{
		shutdownOnce: oncewait.New(),
	}
	r.controller = &controller{
		root:       r,
		ctx:        parentCtx,
		dependents: &sync.WaitGroup{},
		workers:    &sync.WaitGroup{},
		shutdown:   make(chan struct{}),
	}

	// trap cancellation of Context or calling ShutdownFunc.
	done := make(chan struct{})
	go func() {
		<-parentCtx.Done()
		r.wait()
		close(done)
		return
	}()

	return r, func(ctx context.Context) error {
		// determine shutdown context.
		shutdownCtx, cancelShutdown := r.determineShutdownContext(ctx)
		defer cancelShutdown()

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

// WithDefaultShutdownContext returns copy of parent which has a value of newShutdownCtx.
// When Controller starts shutdown, if shutdown context is not specified, Controller calls newShutdownCtx and use return value as shutdown Context.
// It is intended to be used when starting shutdown by canceling Context.
func WithDefaultShutdownContext(ctx context.Context, newShutdownCtx func(ctx context.Context) context.Context) context.Context {
	return context.WithValue(ctx, defaultShutdownKey, func(ctx context.Context) (context.Context, context.CancelFunc) {
		return newShutdownCtx(ctx), func() {}
	})
}

// WithDefaultShutdownTimeout is a shorthand of
//	WithDefaultShutdownContext(ctx, func(ctx context.Context) context.Context {
//		return context.WithTimeout(ctx, timeout)
//	})
func WithDefaultShutdownTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, defaultShutdownKey, func(ctx context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, timeout)
	})
}

// WithAbort set Aborter to new Context based on ctx.
// Call Abort with returned new Context, set Aborter's Abort is called.
func WithAbort(ctx context.Context, a *Aborter) context.Context {
	return context.WithValue(ctx, abortKey, a)
}

// Abort invoke Aborter.Abort set by using WithAbort.
// Workers can signal abort if they know Controller's context.
func Abort(ctx context.Context) {
	v := ctx.Value(abortKey)
	if v != nil {
		v.(*Aborter).Abort()
	}
}

// Determine context of shutdown.
// When ShutdownFunc is called, parameter 'ctx' is used.
// However, when context of root Controller is canceled, we have no context, so we have to determine shutdown context.
// Unless WithDefaultShutdownContext or WithDefaultShutdownTimeout is set, context.Background() is used.
func (r *root) determineShutdownContext(ctx context.Context) (context.Context, context.CancelFunc) {
	r.shutdownOnce.Do(func() {
		if ctx != nil {
			r.shutdownCtx = ctx
			r.cancelShutdown = func() {}
		} else {
			v := r.ctx.Value(defaultShutdownKey)
			if newShutdownCtx, ok := v.(func(context.Context) (context.Context, context.CancelFunc)); ok {
				r.shutdownCtx, r.cancelShutdown = newShutdownCtx(context.Background())
			} else {
				r.shutdownCtx = context.Background()
				r.cancelShutdown = func() {}
			}
		}
	})
	return r.shutdownCtx, r.cancelShutdown
}

var _ Controller = (*root)(nil)

type controller struct {
	root       *root
	ctx        context.Context
	dependents *sync.WaitGroup
	workers    *sync.WaitGroup
	shutdown   chan struct{}
	rcs        Closer
	m          sync.Mutex
}

// to shut down Controller
// 	1. wait all dependents shut down
//	2. signal workers to start shut down
//	3. wait all workers shut down
//	4. close all bound resources
func (c *controller) wait() {
	c.dependents.Wait()
	close(c.shutdown)
	c.workers.Wait()
	_ = c.rcs.Close()
	return
}

// Launch launches worker, and set trap to catch signal of shutdown.
func (c *controller) Launch(l WorkerLauncher) error {
	c.m.Lock()
	defer c.m.Unlock()

	// prevent launch after shutdown.
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
	}

	stop, err := l.LaunchWorker(c.ctx)
	if err != nil {
		return err
	}

	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		<-c.shutdown

		ctx, _ := c.root.determineShutdownContext(nil)
		stop(ctx)
		return
	}()
	return nil
}

// Dependent creates dependent Controller, and set trap to catch signal of shutdown.
func (c *controller) Dependent() Controller {
	dependent := &controller{
		root:       c.root,
		ctx:        c.ctx,
		dependents: &sync.WaitGroup{},
		workers:    &sync.WaitGroup{},
		shutdown:   make(chan struct{}),
	}
	c.dependents.Add(1)
	go func() {
		defer c.dependents.Done()
		select {
		case <-c.ctx.Done():
			dependent.wait()
		}
		return
	}()
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
		root:       c.root,
		ctx:        ctx,
		dependents: c.dependents,
		workers:    c.workers,
		rcs:        c.rcs,
	}
}

var _ Controller = (*controller)(nil)
