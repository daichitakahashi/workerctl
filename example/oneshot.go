package main

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/daichitakahashi/workerctl"
)

type OneShotTaskRunner struct {
	r      *workerctl.JobRunner
	conn   net.Conn
	Writer io.Writer
}

func (o *OneShotTaskRunner) LaunchWorker(_ context.Context) (func(context.Context), error) {
	conn, err := net.Dial("tcp", "*.*.*.*")
	if err != nil {
		return nil, err
	}
	o.conn = conn
	o.r = &workerctl.JobRunner{}

	// stop func
	return func(ctx context.Context) {
		defer func() {
			_ = o.conn.Close()
		}()
		err := o.r.Wait(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(o.Writer, "OneShotTaskRunner.Shutdown: %s\n", err)
		}
	}, nil
}

func (o *OneShotTaskRunner) Report(message string) {
	o.r.Run(context.Background(), func(ctx context.Context) {
		var err error
		defer func() {
			if err != nil {
				_, _ = fmt.Fprintf(o.Writer, "OneShotTaskRunner.Report: %s\n", err)
			}
		}()

		select {
		case <-ctx.Done():
			err = ctx.Err()
			return
		default:
		}

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
