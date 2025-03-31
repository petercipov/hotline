package main

import (
	"app/setup"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	systemTime := setup.NewSystemTime()
	app, appErr := setup.NewApp(&setup.Config{
		OtelHttpReporter: struct {
			Secured bool
			Host    string
		}{Secured: false, Host: "localhost:4318"},
		OtelHttpIngestion: struct{ Host string }{
			Host: "localhost:8080",
		},
		SloPipeline: struct{ CheckPeriod time.Duration }{
			CheckPeriod: 10 * time.Second,
		},
	}, systemTime, setup.NewGoHttpServer)

	if appErr != nil {
		panic(appErr)
	}

	graceShuthdown := make(chan os.Signal, 1)
	signal.Notify(graceShuthdown, os.Interrupt, syscall.SIGTERM)
	app.Start()
	<-graceShuthdown
	slog.Info("Shutting down")
	_ = app.Stop()
}
