package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daichitakahashi/workerctl"
)

func main() {
	ctl := workerctl.New(context.Background())
	a := workerctl.NewAbort()

	_ = ctl.NewWorkerGroup("output", func(ctx context.Context, group *workerctl.WorkerGroup) (_ func(context.Context) error, err error) {
		var closer workerctl.Closer
		defer func() {
			if err != nil {
				_ = closer.Close(nil)
			}
		}()

		db, err := func() (conn *sql.Conn, err error) { return }()
		if err != nil {
			return nil, err
		}
		closer.Append(db)

		dst, err := func() (file *os.File, err error) { return }()
		if err != nil {
			return nil, err
		}
		closer.Append(dst)

		_ = group.NewJobRunner("log", func(ctx context.Context, runner *workerctl.JobRunner) (func(context.Context) error, error) {
			return runner.Stop, nil
		})
		return func(ctx context.Context) (err error) {
			defer func() {
				err = closer.Close(err)
			}()
			return group.Stop(ctx)
		}, nil
	})

	_ = ctl.NewWorker("server", func(ctx context.Context) (func(context.Context) error, error) {
		mux := http.NewServeMux()
		mux.Handle("/sleep", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO:
		}))
		svr := http.Server{
			Handler: mux,
		}

		// abort when ListenAndServe fail
		go a.Abort(
			workerctl.PanicSafe(func() error {
				err := svr.ListenAndServe()
				if err == http.ErrServerClosed {
					return nil
				}
				return err
			}),
		)

		return func(ctx context.Context) error {
			select {
			case <-a.Aborted():
				return nil
			default:
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return svr.Shutdown(ctx)
		}, nil
	})

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT)

	select {
	case <-a.Aborted():
		log.Println("Aborted")
	case <-quit:
		log.Println("Signal received")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := ctl.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
}
