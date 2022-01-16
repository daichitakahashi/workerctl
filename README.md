# workerctl: worker controller for graceful shutdown

[![Go Reference](https://pkg.go.dev/badge/github.com/daichitakahashi/workerctl.svg)](https://pkg.go.dev/github.com/daichitakahashi/workerctl)

Package workerctl is controller of application's initialization and its graceful shutdown.

## Usage
```go
import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/daichitakahashi/workerctl"
)

func main() {
	ctx := context.Background()
	ctl, shutdown := workerctl.New(ctx)

	a.AbortOnError(func() (err error) {
		// 1. Init shared resource and bind to this Controller.
		// logOutput is used in both OneShotTaskRunner and Server.
		logOutput, err := os.CreateTemp("", "*log")
		if err != nil {
			return err
		}
		ctl.Bind(logOutput)

		// 2. Start OneShotTaskRunner.
		oneShot := &OneShotTaskRunner{
			Writer: logOutput,
		}
		err = ctl.Launch(oneShot)
		if err != nil {
			return err
		}

		{
			// 3. Create Controller depends on ctl and start Server.
			ctl := ctl.Dependent()

			server := &Server{
				OneShot: oneShot,
				Writer:  logOutput,
			}
			err := ctl.Launch(server)
			if err != nil {
				return err
			}
		}
		return nil
	}())

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT)

	// Once SIGINT received or "/abort" requested, Controller starts shutdown.
	// First, Server's graceful shutdown finished(3).
	// Second, OneShotTaskRunner shutdown after all running task finished(2).
	// Finally, logOutput will be closed(1).
	select {
	case s := <-quit:
		log.Println("signal received:", s)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
}
```
