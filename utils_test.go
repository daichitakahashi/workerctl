package workerctl

import (
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

func TestCloser_Close(t *testing.T) {
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

	err := closer.Close()
	if err == nil || err.Error() != "third" {
		t.Error("unexpected error", err)
		return
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
