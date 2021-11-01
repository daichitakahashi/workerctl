package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/daichitakahashi/workerctl"
)

type Server struct {
	svr *http.Server
	o   *OneShotTaskRunner
	a   *workerctl.Abort
	w   io.Writer
}

func (s *Server) Init(_ context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/report", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(s.w, "%s report\n", r.Method)

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

		s.o.Report(string(message))
	}))

	mux.Handle("/abort", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(s.w, "abort!")
		s.a.Abort()
	}))

	s.svr = &http.Server{Handler: mux}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	err := s.svr.Shutdown(ctx)
	if err != nil {
		log.Println(err)
	}
}
