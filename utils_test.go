package workerctl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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

	err := errors.New("abort")
	a.AbortOnError(err)
	select {
	case <-a.Aborted():
	default:
		t.Error("abort expected but not")
	}

	if !errors.Is(a.Err(), err) {
		t.Errorf("expected '%s' but got '%s'", err, a.Err())
	}

	a.Abort()
}

func ExampleCloser() {
	c, err := func() (c io.Closer, err error) {
		var closer Closer
		defer closer.CloseOnError(err) // close all opened resources if function returns error

		first, err := openFile("first")
		if err != nil {
			return nil, err
		}
		closer = append(closer, first)

		second, err := openFile("second")
		if err != nil {
			return nil, err
		}
		closer = append(closer, second)

		third, err := openFile("third")
		if err != nil {
			return nil, err
		}
		closer = append(closer, third)

		return closer, nil
	}()
	if err != nil {
		log.Fatal(err)
	}

	c.Close()

	// Output:
	// close third
	// close second
	// close first
}

func openFile(s string) (io.Closer, error) {
	return dummyCloser(func() error {
		fmt.Println("close", s)
		return nil
	}), nil
}

func TestTransfer(t *testing.T) {
	holder, cancel := context.WithCancel(
		context.WithValue(context.Background(), 0, "zero"),
	)
	defer cancel()

	ctx := Transfer(context.Background(), holder)

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
