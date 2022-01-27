package workerctl

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

type recorder struct {
	t     *testing.T
	m     sync.Mutex
	lines []string
}

func (r *recorder) record(s string) {
	r.t.Helper()
	r.m.Lock()
	defer r.m.Unlock()
	r.lines = append(r.lines, s)
	// r.t.Log(s)
}

type dummyCloser func() error

func (d dummyCloser) Close() error {
	return d()
}

func TestController(t *testing.T) {
	r := recorder{t: t}
	ctl, shutdown := New(context.Background())

	ctl.Bind(dummyCloser(func() error {
		r.record("close resource 1")
		return nil
	}))

	ctl.Bind(dummyCloser(func() error {
		r.record("close resource 2")
		return nil
	}))

	_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
		r.record("launch worker 1")
		return func(ctx context.Context) {
			r.record("stop worker 1")
		}, nil
	}))

	_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
		r.record("launch worker 2")
		return func(ctx context.Context) {
			time.Sleep(time.Millisecond * 20)
			r.record("stop worker 2")
		}, nil
	}))

	{
		ctl := ctl.Dependent()

		ctl.Bind(dummyCloser(func() error {
			r.record("close resource 3")
			return nil
		}))

		_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
			r.record("launch worker 3")
			return func(ctx context.Context) {
				time.Sleep(time.Millisecond * 20)
				r.record("stop worker 3")
			}, nil
		}))

		_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
			r.record("launch worker 4")
			return func(ctx context.Context) {
				r.record("stop worker 4")
			}, nil
		}))
	}

	{
		ctl := ctl.Dependent()

		_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
			r.record("launch worker 5")
			return func(ctx context.Context) {
				time.Sleep(time.Millisecond * 10)
				r.record("stop worker 5")
			}, nil
		}))
	}

	err := shutdown(nil)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"launch worker 1",
		"launch worker 2",
		"launch worker 3",
		"launch worker 4",
		"launch worker 5",
		"stop worker 4",
		"stop worker 5",
		"stop worker 3",
		"close resource 3",
		"stop worker 1",
		"stop worker 2",
		"close resource 2",
		"close resource 1",
	}

	if diff := cmp.Diff(expected, r.lines); diff != "" {
		t.Fatal(diff)
	}
}

func TestController_Cancel(t *testing.T) {
	r := recorder{t: t}

	ctx, cancel := context.WithCancel(context.Background())
	ctl, _ := New(ctx)

	ctl.Bind(dummyCloser(func() error {
		r.record("close resource 1")
		return nil
	}))

	ctl.Bind(dummyCloser(func() error {
		r.record("close resource 2")
		return nil
	}))

	_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
		r.record("launch worker 1")
		return func(ctx context.Context) {
			r.record("stop worker 1")
		}, nil
	}))

	_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
		r.record("launch worker 2")
		return func(ctx context.Context) {
			time.Sleep(time.Millisecond * 20)
			r.record("stop worker 2")
		}, nil
	}))

	{
		ctl := ctl.Dependent()

		ctl.Bind(dummyCloser(func() error {
			r.record("close resource 3")
			return nil
		}))

		_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
			r.record("launch worker 3")
			return func(ctx context.Context) {
				time.Sleep(time.Millisecond * 20)
				r.record("stop worker 3")
			}, nil
		}))

		_ = ctl.Launch(Func(func(ctx context.Context) (func(ctx context.Context), error) {
			r.record("launch worker 4")
			return func(ctx context.Context) {
				r.record("stop worker 4")
			}, nil
		}))
	}

	cancel()
	time.Sleep(time.Millisecond * 500)

	expected := []string{
		"launch worker 1",
		"launch worker 2",
		"launch worker 3",
		"launch worker 4",
		"stop worker 4",
		"stop worker 3",
		"close resource 3",
		"stop worker 1",
		"stop worker 2",
		"close resource 2",
		"close resource 1",
	}

	if diff := cmp.Diff(expected, r.lines); diff != "" {
		t.Fatal(diff)
	}
}
