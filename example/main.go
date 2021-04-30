package main

import (
	"context"
	"io"
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
	abort := make(chan struct{})

	_ = ctl.NewWorkerGroup("output", func(ctx context.Context, group *workerctl.WorkerGroup) (func(context.Context) error, error) {
		var err error
		var c workerctl.Closer
		defer func() {
			if err != nil {
				_ = c.Close()
			}
		}()

		db, err := func() (closer io.Closer, err error) { return }()
		if err != nil {
			return nil, err
		}
		c.Append(db.Close)

		redis, err := func() (closer io.Closer, err error) { return }()
		if err != nil {
			return nil, err
		}
		c.Append(redis.Close)

		_ = group.NewJobRunner("log", func(ctx context.Context, runner *workerctl.JobRunner) (func(context.Context) error, error) {
			return runner.Stop, nil
		})
		return func(ctx context.Context) error {
			defer func() {
				_ = c.Close()
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

		workerctl.Go(func() {
			err := svr.ListenAndServe()
			if err != nil && err != http.ErrServerClosed {
				log.Println(err)
				close(abort)
			}
		})

		return func(ctx context.Context) error {
			select {
			case <-abort:
				return nil
			default:
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return svr.Shutdown(ctx)
		}, nil
	})

	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT)

	select {
	case <-abort:
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
