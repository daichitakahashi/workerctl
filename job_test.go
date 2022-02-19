package workerctl_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/daichitakahashi/workerctl"
)

func TestJobRunner(t *testing.T) {
	var runner workerctl.JobRunner
	var completed int32

	runNum := int32(50)
	for i := int32(0); i < runNum; i++ {
		runner.Go(func() {
			time.Sleep(time.Millisecond * 20 * time.Duration(i+1))
			atomic.AddInt32(&completed, 1)
		})
	}

	err := runner.Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if n := atomic.LoadInt32(&completed); n != runNum {
		t.Fatalf("expected completed count = %d, buf actual count = %d", runNum, n)
	}
}

func TestJobRunner_Panic(t *testing.T) {
	t.Run("without PanicHandler", func(t *testing.T) {
		var runner workerctl.JobRunner
		defer func() {
			rvr := recover()
			if rvr == nil {
				t.Fatal("panic expected but not")
			}
		}()
		runner.Run(func() {
			panic("panic!")
		})
	})

	t.Run("without PanicHandler", func(t *testing.T) {
		var runner workerctl.JobRunner
		defer func() {
			rvr := recover()
			if rvr != nil {
				t.Fatalf("unexpected panic: %v", rvr)
			}
		}()

		var recovered interface{}
		runner.PanicHandler = func(v interface{}) {
			recovered = v
		}

		runner.Run(func() {
			panic("panic!")
		})

		if recovered == nil {
			t.Fatal("panic expected but not")
		}
	})
}

func TestJobRunner_Wait(t *testing.T) {
	var runner workerctl.JobRunner

	runner.Go(func() {
		time.Sleep(time.Millisecond * 500)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*300)
	defer cancel()
	err := runner.Wait(ctx)
	if err == nil {
		t.Fatal("error expected but not")
	} else if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error: %s", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Millisecond*300)
	defer cancel2()
	err = runner.Wait(ctx2)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	runner.Go(func() {
		time.Sleep(time.Millisecond * 500)
	})
	err = runner.Wait(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
}
