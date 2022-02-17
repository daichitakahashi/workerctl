// Package workerctl controls initialization and shutdown of workers that consists of application.
// It aims to describe dependencies of them, and shutdown them in right order.
package workerctl

import (
	"context"
	"errors"
	"io"
	"sync"
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
		Launch(ctx context.Context, l WorkerLauncher) error

		// Bind resources to Controller.
		// After completion of controller's shutdown, resources will be closed.
		Bind(rcs ...io.Closer)
	}

	// ShutdownFunc is a return value of New. It shuts down created Controller.
	// Parameter ctx will be passed to shutdown functions of each worker, so we can set timeout using context.WithDeadline or context.WithTimeout.
	// It is assumed to be used in combination with configuration of container shutdown.
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
	hook        chan struct{}
	shutdownCtx context.Context
}

// New returns a new Controller which can be shut down by calling ShutdownFunc.
func New() (Controller, ShutdownFunc) {
	trap := make(chan struct{})
	r := &root{
		hook: trap,
	}
	r.controller = &controller{
		root:     r,
		shutdown: make(chan struct{}),
	}

	// trap calling ShutdownFunc
	done := make(chan struct{})
	go func() {
		<-trap
		r.wait()
		close(done)
		return
	}()

	return r, func(ctx context.Context) error {
		// set shutdown context
		r.shutdownCtx = ctx

		// hook cancellation
		close(trap)

		select {
		case <-r.shutdownCtx.Done():
			return r.shutdownCtx.Err()
		case <-done:
		}
		return nil
	}
}

var _ Controller = (*root)(nil)

type controller struct {
	root       *root
	dependents sync.WaitGroup
	workers    sync.WaitGroup
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
func (c *controller) Launch(ctx context.Context, l WorkerLauncher) error {
	c.m.Lock()
	defer c.m.Unlock()

	// prevent launch after shutdown.
	select {
	case <-c.root.hook:
		return errors.New("shutdown already started")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	stop, err := l.LaunchWorker(ctx)
	if err != nil {
		return err
	}

	c.workers.Add(1)
	go func() {
		defer c.workers.Done()
		<-c.shutdown
		stop(c.root.shutdownCtx)
		return
	}()
	return nil
}

// Dependent creates dependent Controller, and set trap to catch signal of shutdown.
func (c *controller) Dependent() Controller {
	dependent := &controller{
		root:     c.root,
		shutdown: make(chan struct{}),
	}
	c.dependents.Add(1)
	go func() {
		defer c.dependents.Done()
		<-c.root.hook
		dependent.wait()
		return
	}()
	return dependent
}

func (c *controller) Bind(rcs ...io.Closer) {
	c.m.Lock()
	defer c.m.Unlock()
	c.rcs = append(c.rcs, rcs...)
}

var _ Controller = (*controller)(nil)
