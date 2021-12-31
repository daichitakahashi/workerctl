package workerctl

import (
	"bytes"
	"errors"
	"testing"
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
	var closer Closer
	log := &bytes.Buffer{}
	closer = append(closer, &closePrinter{
		log: log,
		msg: "first",
	})
	closer = append(closer, &closePrinter{
		log: log,
		msg: "second",
	})
	closer = append(closer, &closePrinter{
		log: log,
		msg: "third",
	})

	err := closer.Close()
	if err == nil || err.Error() != "third" {
		t.Error("unexpected error", err)
		return
	} else if log.String() != "third->second->first->" {
		t.Error("unexpected calling order of Close")
		return
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
