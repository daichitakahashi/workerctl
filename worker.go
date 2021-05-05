package workerctl

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"
)

// WorkerGroup :
type WorkerGroup struct {
	c       *Controller
	err     chan error
	ns      string
	m       sync.Mutex
	workers map[string]<-chan error
	option  option
}

func (g *WorkerGroup) launch(name string, fn func(ctx context.Context) (<-chan error, error)) error {
	g.m.Lock()
	defer g.m.Unlock()

	// prevent Launch after Shutdown.
	select {
	case <-g.c.ctx.Done():
		return g.c.ctx.Err()
	default:
	}

	if g.ns != "" {
		name = path.Join(g.ns, name)
	}

	if _, ok := g.workers[name]; ok {
		return fmt.Errorf("worker '%s' already exists", name)
	}
	done, err := fn(g.c.ctx)
	if err != nil {
		return err
	} else if done == nil {
		return errors.New("invalid channel")
	}
	g.workers[name] = done
	return nil
}

func goWithRecover(fn func(), done chan error) {
	go func() {
		defer func() {
			rvr := recover()
			if rvr != nil {
				done <- fmt.Errorf("%v", rvr)
			}
		}()
		fn()
	}()
}

// WorkerFunc :
type WorkerFunc func(ctx context.Context) (onShutdown func(context.Context) error, err error)

// NewWorker :
func (g *WorkerGroup) NewWorker(name string, fn WorkerFunc) error {
	return g.launch(name, func(ctx context.Context) (<-chan error, error) {
		done := make(chan error)

		onShutdown, err := fn(g.c.ctx)
		if err != nil {
			return nil, err
		}
		goWithRecover(func() {
			select {
			case <-g.c.ctx.Done():
				err := onShutdown(g.c.shutdownCtx)
				if err != nil {
					done <- err
					return
				}
				close(done)
			}
		}, done)
		return done, nil
	})
}

// WorkerGroupFunc :
type WorkerGroupFunc func(ctx context.Context, group *WorkerGroup) (onShutdown func(ctx context.Context) error, err error)

// NewWorkerGroup :
func (g *WorkerGroup) NewWorkerGroup(name string, fn WorkerGroupFunc) error {
	return g.launch(name, func(ctx context.Context) (<-chan error, error) {
		group := &WorkerGroup{
			c:       g.c,
			err:     make(chan error),
			ns:      path.Join(g.ns, name),
			workers: make(map[string]<-chan error),
			option: option{
				pollInterval: g.option.pollInterval,
			},
		}
		onShutdown, err := fn(group.c.ctx, group)
		if err != nil {
			return nil, err
		}
		goWithRecover(func() {
			select {
			case <-group.c.ctx.Done():
				err := onShutdown(g.c.shutdownCtx)
				if err != nil {
					group.err <- err
					return
				}
				close(group.err)
			}
		}, group.err)
		return group.err, nil
	})
}

// Stop :
func (g *WorkerGroup) Stop(ctx context.Context, options ...ShutdownOption) error {
	// prevent new worker after shutdown.
	g.m.Lock()
	defer g.m.Unlock()

	// apply options.
	for _, opt := range options {
		opt(&g.option)
	}

	func() {
		ticker := time.NewTicker(g.option.pollInterval)
		defer ticker.Stop()

		for {
			for name, done := range g.workers {
				select {
				case <-ctx.Done():
					return
				case err := <-done:
					if g.option.onWorkerShutdown != nil {
						g.option.onWorkerShutdown(name, err)
					}
					delete(g.workers, name)
				default:
					// continue
				}
			}
			if len(g.workers) == 0 {
				break
			}
			<-ticker.C
		}
	}()

	if len(g.workers) > 0 {
		remains := make([]string, 0, len(g.workers))
		for name := range g.workers {
			remains = append(remains, name)
		}
		return fmt.Errorf("some worker still working: %s", strings.Join(remains, ", "))
	}
	return nil
}

// JobRunner :
type JobRunner struct {
	c            *Controller
	err          chan error
	wg           sync.WaitGroup
	PanicHandler func(interface{})
}

// JobRunnerFunc :
type JobRunnerFunc func(ctx context.Context, runner *JobRunner) (func(context.Context) error, error)

// NewJobRunner :
func (g *WorkerGroup) NewJobRunner(name string, fn JobRunnerFunc) error {
	return g.launch(name, func(ctx context.Context) (<-chan error, error) {
		runner := &JobRunner{
			c:   g.c,
			err: make(chan error),
		}
		onShutdown, err := fn(g.c.ctx, runner)
		if err != nil {
			return nil, err
		}
		goWithRecover(func() {
			select {
			case <-g.c.ctx.Done():
				err := onShutdown(g.c.shutdownCtx)
				if err != nil {
					runner.err <- err
					return
				}
				close(runner.err)
			}
		}, runner.err)
		return runner.err, nil
	})
}

// Go :
func (r *JobRunner) Go(w func(ctx context.Context)) {
	r.wg.Add(1)
	go func() {
		defer func() {
			r.wg.Done()
			if r.PanicHandler != nil {
				rvr := recover()
				if rvr != nil {
					r.PanicHandler(rvr)
				}
			}
		}()
		w(r.c.ctx)
	}()
}

// Stop :
func (r *JobRunner) Stop(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.wg.Wait()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
