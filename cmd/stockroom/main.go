package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/thanhhoa3514/stockroom/internal/application"
	"github.com/thanhhoa3514/stockroom/internal/infrastructure/fakesupplier"
	"github.com/thanhhoa3514/stockroom/internal/infrastructure/sqlite"
	"github.com/thanhhoa3514/stockroom/internal/interfaces/cli"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
func main()                        { os.Exit(run()) }
func run() int {
	config, err := cli.Parse(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var supplier application.Supplier
	if config.Command == "start" || config.Command == "resume" {
		supplier, err = fakesupplier.New(config.Scenario, config.FailBatch)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	db, err := sqlite.Open(config.DB)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()
	clock := systemClock{}
	service := &application.SyncService{UOW: db, Supplier: supplier, Clock: clock}
	if config.Command == "start" || config.Command == "resume" {
		if err := service.Recover(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	app := cli.CLI{Start: application.StartSync{Service: service}, Resume: application.ResumeSync{Service: service}, Status: application.GetSyncStatus{UOW: db, Clock: clock}, Products: application.ListProducts{UOW: db}, Out: os.Stdout}
	if err := app.Run(ctx, config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
