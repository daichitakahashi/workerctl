package workerctl

import (
	"context"
	"log"
	"sync/atomic"
	"time"
)

// Controller is worker controller.
type Controller struct {
	ctx         context.Context
	cancel      context.CancelFunc
	shutdownCtx context.Context
	internal    *WorkerGroup
	aborted     chan struct{}
	abortState  int32
}

// New :
func New(ctx context.Context) *Controller {
	ctl := &Controller{
		aborted: make(chan struct{}),
	}
	ctl.ctx, ctl.cancel = context.WithCancel(ctx)
	ctl.internal = &WorkerGroup{
		c:       ctl,
		err:     make(chan error),
		workers: make(map[string]<-chan error),
		option: option{
			pollInterval: defaultPollInterval,
			log:          log.Println,
		},
	}
	return ctl
}

// NewWorker :
func (c *Controller) NewWorker(name string, fn WorkerFunc) error {
	return c.internal.NewWorker(name, fn)
}

// NewWorkerGroup :
func (c *Controller) NewWorkerGroup(name string, fn WorkerGroupFunc) error {
	return c.internal.NewWorkerGroup(name, fn)
}

// NewJobRunner :
func (c *Controller) NewJobRunner(name string, fn JobRunnerFunc) error {
	return c.internal.NewJobRunner(name, fn)
}

func (c *Controller) Aborted() <-chan struct{} {
	return c.aborted
}

func (c *Controller) Abort(err error) {
	if err != nil {
		_ = c.abort()
	}
}

func (c *Controller) abort() (aborted bool) {
	if atomic.SwapInt32(&c.abortState, 1) == 0 {
		close(c.aborted)
		aborted = true
	}
	return
}

const defaultPollInterval = time.Microsecond * 500

// Shutdown :
func (c *Controller) Shutdown(ctx context.Context, options ...ShutdownOption) error {
	// apply options.
	for _, opt := range options {
		opt(&c.internal.option)
	}

	// set shutdown context.
	if ctx != nil {
		c.shutdownCtx = ctx
	} else {
		c.shutdownCtx = context.Background()
	}

	select {
	case <-c.ctx.Done():
		return nil
	default:
	}
	// cancel main context.
	c.cancel()
	go c.internal.Stop(ctx)

	return <-c.internal.err
}

type option struct {
	pollInterval     time.Duration
	onWorkerShutdown func(name string, err error)
	log              func(v ...interface{})
}

// ShutdownOption :
type ShutdownOption func(*option)

// PollInterval :
func PollInterval(d time.Duration) ShutdownOption {
	return func(o *option) {
		if d >= 0 {
			o.pollInterval = d
		}
	}
}

// OnWorkerShutdown :
func OnWorkerShutdown(fn func(name string, err error)) ShutdownOption {
	return func(o *option) {
		o.onWorkerShutdown = fn
	}
}

// LogFunc :
func LogFunc(f func(v ...interface{})) ShutdownOption {
	return func(o *option) {
		o.log = f
	}
}
