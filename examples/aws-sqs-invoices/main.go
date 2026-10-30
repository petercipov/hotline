// Command aws-sqs-invoices is Hotline's example of a Go app using AWS SQS
// through the proxy. A shop asks for an invoice PDF with "request"; a worker
// receives the job with "work" and writes the PDF. See README.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hotline-example/aws-sqs-invoices/invoice"
)

//nolint:gosec // an example proxy address, not a credential
const usage = `usage:
  aws-sqs-invoices request --order A-1001 --template invoice-standard-v2 --language en [--send-to EMAIL]
  aws-sqs-invoices send --body TEXT
  aws-sqs-invoices work --out FOLDER [--once]

every command also takes:
  --queue URL      the queue's URL (default: $INVOICES_QUEUE_URL)
  --proxy URL      Hotline's forward proxy, with the integration ID as the user name,
                   such as http://invoices:x@hotline:8080 (default: $HOTLINE_PROXY; empty: no proxy)
  --endpoint URL   a local SQS instead of AWS (default: $SQS_ENDPOINT)
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run carries out one command and returns the exit code: 0 done, 1 failed,
// 2 a wrong command line.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	queue := flags.String("queue", os.Getenv("INVOICES_QUEUE_URL"), "")
	proxy := flags.String("proxy", os.Getenv("HOTLINE_PROXY"), "")
	endpoint := flags.String("endpoint", os.Getenv("SQS_ENDPOINT"), "")
	order := flags.String("order", "", "")
	template := flags.String("template", "", "")
	language := flags.String("language", "", "")
	sendTo := flags.String("send-to", "", "")
	body := flags.String("body", "", "")
	out := flags.String("out", "", "")
	once := flags.Bool("once", false, "")

	switch command {
	case "request", "send", "work":
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n%s", command, usage)
		return 2
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	var missing string
	switch {
	case *queue == "":
		missing = "--queue (or INVOICES_QUEUE_URL)"
	case command == "request" && (*order == "" || *template == "" || *language == ""):
		missing = "--order, --template and --language"
	case command == "send" && *body == "":
		missing = "--body"
	case command == "work" && *out == "":
		missing = "--out"
	}
	if missing != "" {
		_, _ = fmt.Fprintf(stderr, "aws-sqs-invoices %s needs %s\n%s", command, missing, usage)
		return 2
	}

	client, err := invoice.NewClient(ctx, invoice.Settings{ProxyURL: *proxy, Endpoint: *endpoint})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	switch command {
	case "request":
		job := invoice.NewJob(*order, *template, *language, *sendTo, time.Now().UTC())
		data, err := json.Marshal(job)
		if err == nil {
			_, err = invoice.Check(data)
		}
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		*body = string(data)
		fallthrough
	case "send":
		id, err := invoice.Send(ctx, client, *queue, *body)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "sent %s\n", id)
	case "work":
		w := &invoice.Worker{Client: client, QueueURL: *queue, OutDir: *out, Log: slog.New(slog.NewJSONHandler(stderr, nil))}
		if err := w.Run(ctx, *once); err != nil && !errors.Is(err, context.Canceled) {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}
