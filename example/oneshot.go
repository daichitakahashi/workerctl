package main

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/daichitakahashi/workerctl"
)

type OneShotTaskRunner struct {
	r    *workerctl.JobRunner
	conn net.Conn
	w    io.Writer
}

func (o *OneShotTaskRunner) Init(_ context.Context) error {
	conn, err := net.Dial("tcp", "*.*.*.*")
	if err != nil {
		return err
	}
	o.conn = conn
	return nil
}

func (o *OneShotTaskRunner) Report(message string) {
	o.r.Run(func() {
		var err error
		defer func() {
			if err != nil {
				_, _ = fmt.Fprintf(o.w, "OneShotTaskRunner.Report: %s\n", err)
			}
		}()

		_, err = o.conn.Write([]byte("ping"))
		if err != nil {
			return
		}
		var pong string
		_, err = fmt.Fscanln(o.conn, &pong)
		if err != nil {
			return
		} else if pong != "pong" {
			err = fmt.Errorf("unkwnon pong: %s", pong)
			return
		}

		_, err = o.conn.Write([]byte(message))
		if err != nil {
			return
		}
		var ok string
		_, err = fmt.Fscanln(o.conn, &ok)
		if err != nil {
			return
		} else if ok != "ok" {
			err = fmt.Errorf("unkown reponse: %s", ok)
		}
	})
}

func (o *OneShotTaskRunner) Shutdown(ctx context.Context) {
	defer func() {
		_ = o.conn.Close()
	}()
	err := o.r.Wait(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(o.w, "OneShotTaskRunner.Shutdown: %s\n", err)
	}
}

var _ workerctl.Worker = (*OneShotTaskRunner)(nil)
