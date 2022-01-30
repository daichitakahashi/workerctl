package workerctl

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPanicSafe(t *testing.T) {
	e := errors.New("error")
	err := PanicSafe(func() error {
		return e
	})
	if err != e {
		t.Error("err is nil or unknown error unexpectedly")
		return
	}

	err = PanicSafe(func() error {
		panic("panic")
		return e
	})
	if r, ok := err.(*RecoveredError); !ok {
		t.Error("recovered v expected but not")
	} else if r.Error() != `recovered: panic` {
		t.Error("unknown recovered value")
	}
}

func TestAborter(t *testing.T) {
	var a Aborter
	select {
	case <-a.Aborted():
		t.Error("unexpected abort")
	default:
	}

	a.AbortOnError(nil)
	select {
	case <-a.Aborted():
		t.Error("unexpected abort")
	default:
	}

	a.AbortOnError(errors.New("abort"))
	select {
	case <-a.Aborted():
	default:
		t.Error("abort expected but not")
	}
}

func TestCloser(t *testing.T) {
	r := recorder{t: t}

	var closer Closer
	closer = append(closer, dummyCloser(func() error {
		msg := "first"
		r.record(msg)
		return errors.New(msg)
	}))
	closer = append(closer, dummyCloser(func() error {
		msg := "second"
		r.record(msg)
		return errors.New(msg)
	}))
	closer = append(closer, dummyCloser(func() error {
		msg := "third"
		r.record(msg)
		return errors.New(msg)
	}))

	err := closer.CloseOnError(nil)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
		return
	} else if len(r.lines) > 0 {
		t.Error("no close expected but not: ", r.lines)
		return
	}

	err = closer.CloseOnError(errors.New("cause"))
	if err == nil {
		t.Error("error expected but not")
	}

	expected := []string{
		"third",
		"second",
		"first",
	}

	if diff := cmp.Diff(expected, r.lines); diff != "" {
		t.Error(diff)
		return
	}
}

func TestTransferContext(t *testing.T) {
	holder, cancel := context.WithCancel(
		context.WithValue(context.Background(), 0, "zero"),
	)
	defer cancel()

	ctx := TransferContext(context.Background(), holder)

	s, ok := ctx.Value(0).(string)
	if !ok || s != "zero" {
		t.Error(`expected to get string value "zero" with key '0'`)
		return
	}

	cancel()
	select {
	case <-ctx.Done():
		t.Error("unexpected cancellation")
		return
	default:
		// ok
	}
}
