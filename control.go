// Package workerctl implements controller of worker
// アプリケーションを構成するワーカーの起動とシャットダウンをコントロールする。
// ワーカーの依存関係を記述し、それらを適切な順序でシャットダウンさせることを目的としている。
package workerctl

import (
	"context"
	"io"
	"sync"
	"time"
)

type (
	// Controller is core interface of workerctl package.
	// ワーカーの起動のほか、現在の Controller に依存する Controller を生成することで、依存関係を構築することができる。
	// シャットダウンでは派生した Controller が先にシャットダウンし、その後に派生元の Controller のシャットダウンが開始される。
	// Controller と起動したワーカーは関連づけられ、 関連づけられた Controller がシャットダウンするフェーズになって初めてワーカーのシャットダウンが行われる。
	// io.Closer を満たすリソースをバインドすることで、関連づけたワーカーのシャットダウン後にそれらを Close させることができる。
	Controller interface {
		// Dependent creates new Controller depends on parent.
		// 派生した Controller が全てシャットダウンした後に、派生元の Controller はのシャットダウンが開始される。
		Dependent() Controller

		// Launch registers WorkerLauncher to this Controller and call it.
		// Return error when LaunchWorker cause error.
		// ワーカー起動時にエラーが発生した場合は、単にそのワーカーが起動しないだけで、 Controller の状態に影響を及ぼすことはない。
		// エラーによって依存性の記述に失敗した場合、明示的にシャットダウンさせない限り、起動に成功したワーカーの終了とリソースの解放が行われることはない。
		Launch(l WorkerLauncher) error

		// Bind resource to Controller.
		// After completion of controller's shutdown, resources will be closed.
		Bind(rc io.Closer)

		// Context returns Controller's Context.
		Context() context.Context

		// WithContext returns a Controller with its context changed to ctx.
		// The provided ctx must be non-nil.
		// 主に context.WithValue との併用を想定している。
		WithContext(ctx context.Context) Controller
	}

	// ShutdownFunc is a return value of New.
	// ctx は各ワーカーのシャットダウン関数に渡されるため、タイムアウトをセットすることで正常なシャットダウンの時間制限を課すことができる。
	// `docker stop --timeout`, AWS ECSのタスク定義パラメータ`stopTimeout` 等と組み合わせて使用することを想定。
	// 各ワーカーのシャットダウンに残された時間は、 context.Context.Deadline を参照のこと。
	ShutdownFunc func(ctx context.Context) error

	// WorkerLauncher is responsible for initializing the worker and returning its shutdown function.
	// ワーカーの起動に失敗した場合には、起動中に初期化したリソースを全て解放したうえで error を返却することが期待されている。
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

// rootは New で作成される Controller の正体で、派生する全てのコントローラーの大元となる。
type root struct {
	*controller
	shutdownCtx    context.Context
	cancelShutdown context.CancelFunc
	shutdownOnce   sync.Once
	determined     chan struct{}
}

// New returns a new Controller which scope is bound to ctx and ShutdownFunc.
// ctxがタイムアウトするかキャンセルされた場合のシャットダウン Context を設定したい場合には、
// ctxに対して WithDefaultShutdownContext を使用する。
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

// WithDefaultShutdownContext は、ctxに Controller のシャットダウンで使用するデフォルトの Context を生成する関数をセットすることができる。
// デフォルトのシャットダウンコンテクストは、 Controller の親コンテクストがタイムアウトするかキャンセルされた場合に使用され、
// 明示的に ShutdownFunc をコールしてシャットダウンコンテクストが渡された場合はそちらを優先して使用する。
func WithDefaultShutdownContext(ctx context.Context, newShutdownCtx func(ctx context.Context) context.Context) context.Context {
	return context.WithValue(ctx, defaultShutdownKey, func(ctx context.Context) (context.Context, context.CancelFunc) {
		return newShutdownCtx(ctx), func() {}
	})
}

// WithDefaultShutdownTimeout は、ctxに Controller のシャットダウンで使用するデフォルトの Context にタイムアウトをセットするフックをセットすることができる。
// WithDefaultShutdownContext と併用することはできない。
func WithDefaultShutdownTimeout(ctx context.Context, timeout time.Duration) context.Context {
	return context.WithValue(ctx, defaultShutdownKey, func(ctx context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(ctx, timeout)
	})
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
		close(r.determined)
	})
	<-r.determined
	return r.shutdownCtx, r.cancelShutdown
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
			ctx, _ := c.root.determineShutdownContext(nil)
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
