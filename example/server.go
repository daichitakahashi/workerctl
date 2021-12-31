package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"

	"github.com/daichitakahashi/workerctl"
)

type Server struct {
	OneShot *OneShotTaskRunner
	Writer  io.Writer
}

func (s *Server) LaunchWorker(ctx context.Context) (func(context.Context), error) {
	mux := http.NewServeMux()
	mux.Handle("/report", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(s.Writer, "%s report\n", r.Method)

		if r.Method != http.MethodPost {
			http.Error(w, "unavailable", http.StatusMethodNotAllowed)
			return
		}

		message, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read message", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "received",
		})

		s.OneShot.Report(string(message))
	}))

	mux.Handle("/abort", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(s.Writer, "abort!")
		workerctl.Abort(ctx)
	}))

	svr := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	svr.BaseContext = func(_ net.Listener) context.Context {
		return ctx
	}

	go func() {
		err := workerctl.PanicSafe(func() error {
			return svr.ListenAndServe()
		})
		if err != nil && err != http.ErrServerClosed {
			log.Println(err)
			workerctl.Abort(ctx)
		}
	}()

	// stop func
	return func(ctx context.Context) {
		err := svr.Shutdown(ctx)
		if err != nil {
			log.Println(err)
		}
	}, nil
}

var _ workerctl.WorkerLauncher = (*Server)(nil)
