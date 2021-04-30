package workerctl

import (
	"context"
	"log"
	"time"
)

// Controller is worker controller.
type Controller struct {
	ctx         context.Context
	cancel      context.CancelFunc
	shutdownCtx context.Context
	internal    *WorkerGroup
}

// New :
func New(ctx context.Context) *Controller {
	ctl := &Controller{}
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

const defaultPollInterval = time.Microsecond * 500

// ShutdownCtx :
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
