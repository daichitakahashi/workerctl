package workerctl

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestJobRunner(t *testing.T) {
	var runner JobRunner
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
		var runner JobRunner
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
		var runner JobRunner
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
