package workerctl

import (
	"context"
	"time"
)

// DefaultPollInterval :
const DefaultPollInterval = time.Millisecond * 50

// DefaultSeparator :
const DefaultSeparator = "/"

// Controller is worker controller.
type Controller struct {
	ctx         context.Context
	cancel      context.CancelFunc
	shutdownCtx context.Context
	internal    *WorkerGroup
	option      option
}

// New :
func New(ctx context.Context, options ...Option) *Controller {
	ctl := &Controller{
		option: option{
			pollInterval: DefaultPollInterval,
			sep:          DefaultSeparator,
		},
	}
	ctl.ctx, ctl.cancel = context.WithCancel(ctx)

	// apply options.
	for _, opt := range options {
		opt(&ctl.option)
	}

	ctl.internal = &WorkerGroup{
		c:       ctl,
		err:     make(chan error),
		workers: make(map[string]<-chan error),
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

// Shutdown :
func (c *Controller) Shutdown(ctx context.Context) error {
	// set shutdown context.
	if ctx != nil {
		c.shutdownCtx = ctx
	} else {
		c.shutdownCtx = context.Background()
	}

	select {
	case <-c.shutdownCtx.Done():
		return c.shutdownCtx.Err()
	default:
	}
	// cancel main context.
	c.cancel()
	return c.internal.Wait(c.shutdownCtx)
}

type option struct {
	pollInterval     time.Duration
	sep              string
	onWorkerShutdown func(name string, err error)
}

// Option :
type Option func(*option)

// PollInterval :
func PollInterval(d time.Duration) Option {
	return func(o *option) {
		if d > 0 {
			o.pollInterval = d
		}
	}
}

// NamespaceSeparator :
func NamespaceSeparator(sep string) Option {
	return func(o *option) {
		o.sep = sep
	}
}

// OnWorkerShutdown :
func OnWorkerShutdown(fn func(name string, err error)) Option {
	return func(o *option) {
		o.onWorkerShutdown = fn
	}
}
