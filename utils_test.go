package workerctl_test

import (
	"errors"
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/daichitakahashi/workerctl"
)

func TestPanicSafe(t *testing.T) {
	e := errors.New("error")
	err := workerctl.PanicSafe(func() error {
		return e
	})
	if err != e {
		t.Error("err is nil or unknown error unexpectedly")
		return
	}

	err = workerctl.PanicSafe(func() error {
		panic("panic")
		return e
	})
	if r, ok := err.(*workerctl.RecoveredError); !ok {
		t.Error("recovered v expected but not")
	} else if r.Error() != `recovered: panic` {
		t.Error("unknown recovered value")
	}
}

func TestAborter(t *testing.T) {
	var a workerctl.Aborter
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
		var closer workerctl.Closer
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
