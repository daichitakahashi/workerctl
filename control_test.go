package workerctl_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/daichitakahashi/workerctl"
)

type dummyCloser func() error

func (d dummyCloser) Close() error {
	return d()
}

func ExampleController() {
	ctx := context.Background()
	ctl, shutdown := workerctl.New()

	ctl.Bind(dummyCloser(func() error {
		fmt.Println("close resource 1")
		return nil
	}))

	ctl.Bind(dummyCloser(func() error {
		fmt.Println("close resource 2")
		return nil
	}))

	_ = ctl.Launch(ctx, workerctl.Func(func(ctx context.Context) (func(ctx context.Context), error) {
		fmt.Println("launch worker 1")
		return func(ctx context.Context) {
			fmt.Println("stop worker 1")
		}, nil
	}))

	_ = ctl.Launch(ctx, workerctl.Func(func(ctx context.Context) (func(ctx context.Context), error) {
		fmt.Println("launch worker 2")
		return func(ctx context.Context) {
			time.Sleep(time.Millisecond * 20)
			fmt.Println("stop worker 2")
		}, nil
	}))

	{
		ctl := ctl.Dependent()

		ctl.Bind(dummyCloser(func() error {
			fmt.Println("close resource 3")
			return nil
		}))

		_ = ctl.Launch(ctx, workerctl.Func(func(ctx context.Context) (func(ctx context.Context), error) {
			fmt.Println("launch worker 3")
			return func(ctx context.Context) {
				time.Sleep(time.Millisecond * 20)
				fmt.Println("stop worker 3")
			}, nil
		}))

		_ = ctl.Launch(ctx, workerctl.Func(func(ctx context.Context) (func(ctx context.Context), error) {
			fmt.Println("launch worker 4")
			return func(ctx context.Context) {
				fmt.Println("stop worker 4")
			}, nil
		}))
	}

	{
		ctl := ctl.Dependent()

		_ = ctl.Launch(ctx, workerctl.Func(func(ctx context.Context) (func(ctx context.Context), error) {
			fmt.Println("launch worker 5")
			return func(ctx context.Context) {
				time.Sleep(time.Millisecond * 10)
				fmt.Println("stop worker 5")
			}, nil
		}))
	}

	err := shutdown(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	// Output:
	// launch worker 1
	// launch worker 2
	// launch worker 3
	// launch worker 4
	// launch worker 5
	// stop worker 4
	// stop worker 5
	// stop worker 3
	// close resource 3
	// stop worker 1
	// stop worker 2
	// close resource 2
	// close resource 1
}
