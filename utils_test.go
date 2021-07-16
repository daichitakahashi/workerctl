package workerctl

import (
	"bytes"
	"errors"
	"testing"
)

func TestGo(t *testing.T) {
	done := make(chan struct{})
	Go(func() {
		done <- struct{}{}
	}, func(v interface{}) {
		t.Error("not expected recover call")
	})
	<-done

	Go(func() {
		panic("panic")
	}, func(v interface{}) {
		if s, ok := v.(string); !ok || s != "panic" {
			t.Error("not expected panic")
		}
		done <- struct{}{}
	})
	<-done
}

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

func TestNewAbort_Abort(t *testing.T) {
	a := NewAbort()
	select {
	case <-a.Aborted():
		t.Error("unexpected abort")
	default:
	}

	a.Abort(nil)
	select {
	case <-a.Aborted():
		t.Error("unexpected abort")
	default:
	}

	a.Abort(errors.New("abort"))
	select {
	case <-a.Aborted():
	default:
		t.Error("abort expected but not")
	}
}

func TestCloser_Close(t *testing.T) {
	var closer Closer
	log := &bytes.Buffer{}
	closer.Append(&closePrinter{
		log: log,
		msg: "first",
	})
	closer.Append(&closePrinter{
		log: log,
		msg: "second",
	})
	closer.Append(&closePrinter{
		log: log,
		msg: "third",
	})

	err := closer.Close(nil)
	if err == nil || err.Error() != "third" {
		t.Error("unexpected error", err)
		return
	} else if log.String() != "third->second->first->" {
		t.Error("unexpected calling order of Close")
		return
	}

	e := errors.New("error")
	err = closer.Close(e)
	if err != e {
		t.Error("unexpected error", err)
	}
}

type closePrinter struct {
	log *bytes.Buffer
	msg string
}

func (c *closePrinter) Close() error {
	c.log.WriteString(c.msg + "->")
	return errors.New(c.msg)
}
