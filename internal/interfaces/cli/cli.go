package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/thanhhoa3514/stockroom/internal/application"
)

const Usage = `Usage: stockroom [--db PATH] COMMAND [OPTIONS]
Commands:
  start    [--scenario basic|repeat|late-old|bad-data|outage] [--fail-batch N]
  resume   [--scenario NAME]
  status   [--run ID]
  products [--status active|withdrawn]
`

type Config struct {
	DB, Command, Scenario, Status string
	RunID                         int64
	FailBatch                     int
}

func Parse(args []string, errOut io.Writer) (Config, error) {
	c := Config{}
	root := flag.NewFlagSet("stockroom", flag.ContinueOnError)
	root.SetOutput(errOut)
	root.Usage = func() { fmt.Fprint(errOut, Usage) }
	root.StringVar(&c.DB, "db", "./stockroom.db", "SQLite database path")
	if err := root.Parse(args); err != nil {
		return c, err
	}
	rest := root.Args()
	if len(rest) == 0 {
		return c, fmt.Errorf("command required\n%s", Usage)
	}
	c.Command = rest[0]
	sub := flag.NewFlagSet(c.Command, flag.ContinueOnError)
	sub.SetOutput(errOut)
	switch c.Command {
	case "start", "resume":
		sub.StringVar(&c.Scenario, "scenario", "basic", "fictional supplier scenario")
		if c.Command == "start" {
			sub.IntVar(&c.FailBatch, "fail-batch", 2, "one-based failing batch in outage; 0 disables failure")
		}
	case "status":
		sub.Int64Var(&c.RunID, "run", 0, "run ID; default latest")
	case "products":
		sub.StringVar(&c.Status, "status", "", "active or withdrawn")
	default:
		return c, fmt.Errorf("unknown command %q\n%s", c.Command, Usage)
	}
	if err := sub.Parse(rest[1:]); err != nil {
		return c, err
	}
	if sub.NArg() != 0 {
		return c, fmt.Errorf("unexpected arguments: %s", strings.Join(sub.Args(), " "))
	}
	if c.RunID < 0 {
		return c, fmt.Errorf("run ID must be positive")
	}
	if c.FailBatch < 0 {
		return c, fmt.Errorf("fail-batch must not be negative")
	}
	if c.Status != "" && c.Status != "active" && c.Status != "withdrawn" {
		return c, fmt.Errorf("status must be active or withdrawn")
	}
	return c, nil
}

type CLI struct {
	Start    application.StartSync
	Resume   application.ResumeSync
	Status   application.GetSyncStatus
	Products application.ListProducts
	Out      io.Writer
}

func (c CLI) Run(ctx context.Context, config Config) error {
	switch config.Command {
	case "start", "resume":
		var out application.SyncOutput
		var err error
		if config.Command == "start" {
			out, err = c.Start.Execute(ctx, application.SyncInput{})
		} else {
			out, err = c.Resume.Execute(ctx, application.SyncInput{})
		}
		if out.RunID != 0 {
			fmt.Fprintf(c.Out, "Run: %d\n", out.RunID)
		}
		if err != nil {
			if out.RunID != 0 {
				fmt.Fprintln(c.Out, "INCOMPLETE: resume with `stockroom resume`")
			}
			return err
		}
		fmt.Fprintln(c.Out, "COMPLETED")
		return nil
	case "status":
		s, err := c.Status.Execute(ctx, application.StatusInput{RunID: config.RunID})
		if err != nil {
			return err
		}
		fmt.Fprintf(c.Out, "Run: %d\nState: %s\nNext supplier cursor: %q\nStarted: %s\nFinished: %s\nCreated: %d\nUpdated: %d\nWithdrawn: %d\nInvalid: %d\n", s.RunID, strings.ToUpper(s.Status), s.Cursor, displayTime(s.StartedAt), displayTime(s.FinishedAt), s.Counts.Created, s.Counts.Updated, s.Counts.Withdrawn, s.InvalidCount)
		if s.Status != "completed" {
			fmt.Fprintln(c.Out, "INCOMPLETE: resume with `stockroom resume`")
		}
		if s.Error != "" {
			fmt.Fprintf(c.Out, "Error: %s\n", s.Error)
		}
		for _, issue := range s.Issues {
			fmt.Fprintf(c.Out, "Issue: batch=%q sku=%q reason=%q\n", issue.BatchID, issue.SKU, issue.Reason)
		}
		return nil
	case "products":
		ps, err := c.Products.Execute(ctx, application.ProductsInput{Status: config.Status})
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(c.Out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SKU\tNAME\tPRICE_CENTS\tSTATUS\tVERSION\tBATCH\tPREVIOUS_BATCH\tCREATED_AT\tUPDATED_AT")
		for _, p := range ps {
			fmt.Fprintf(w, "%s\t%q\t%d\t%s\t%d\t%s\t%s\t%s\t%s\n", p.SKU, p.Name, p.PriceCents, p.Status, p.Version, p.BatchID, p.PreviousBatchID, displayTime(p.CreatedAt), displayTime(p.UpdatedAt))
		}
		return w.Flush()
	default:
		return fmt.Errorf("unknown command %q", config.Command)
	}
}
func displayTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339Nano)
}
