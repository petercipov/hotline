// Command aws-sqs-invoices is Hotline's example of a Go app using AWS SQS
// and S3 through the proxy. A shop asks for an invoice PDF with "request";
// a worker receives the job with "work" and writes the PDF, to a folder or
// to an S3 bucket; the shop fetches, lists and cleans up the PDFs in the
// bucket. See README.md.
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
  aws-sqs-invoices work (--out FOLDER | --bucket BUCKET) [--once]
  aws-sqs-invoices fetch --order A-1001 --out FILE
  aws-sqs-invoices list
  aws-sqs-invoices cleanup --before 2026-11-01

every command also takes:
  --queue URL        the queue's URL (default: $INVOICES_QUEUE_URL)
  --bucket NAME      the S3 bucket for invoice PDFs (default: $INVOICES_BUCKET)
  --proxy URL        Hotline's forward proxy, with the integration ID as the user name,
                     such as http://invoices:x@hotline:8080 (default: $HOTLINE_PROXY; empty: no proxy)
  --endpoint URL     a local SQS instead of AWS (default: $SQS_ENDPOINT)
  --s3-endpoint URL  a local S3 instead of AWS (default: $S3_ENDPOINT)
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
	bucket := flags.String("bucket", os.Getenv("INVOICES_BUCKET"), "")
	proxy := flags.String("proxy", os.Getenv("HOTLINE_PROXY"), "")
	endpoint := flags.String("endpoint", os.Getenv("SQS_ENDPOINT"), "")
	s3Endpoint := flags.String("s3-endpoint", os.Getenv("S3_ENDPOINT"), "")
	order := flags.String("order", "", "")
	template := flags.String("template", "", "")
	language := flags.String("language", "", "")
	sendTo := flags.String("send-to", "", "")
	body := flags.String("body", "", "")
	out := flags.String("out", "", "")
	before := flags.String("before", "", "")
	once := flags.Bool("once", false, "")

	switch command {
	case "request", "send", "work", "fetch", "list", "cleanup":
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n%s", command, usage)
		return 2
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if missing := missingFlags(command, *queue, *bucket, *order, *template, *language, *body, *out, *before); missing != "" {
		_, _ = fmt.Fprintf(stderr, "aws-sqs-invoices %s needs %s\n%s", command, missing, usage)
		return 2
	}
	cutoff, err := time.Parse(time.DateOnly, *before)
	if command == "cleanup" && err != nil {
		_, _ = fmt.Fprintf(stderr, "aws-sqs-invoices cleanup needs --before as a day, such as 2026-11-01\n%s", usage)
		return 2
	}

	var files *invoice.Files
	if *bucket != "" {
		client, err := invoice.NewFilesClient(ctx, invoice.Settings{ProxyURL: *proxy, Endpoint: *s3Endpoint})
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		files = &invoice.Files{Client: client, Bucket: *bucket}
	}
	switch command {
	case "fetch", "list", "cleanup":
		err = bucketCommand(ctx, command, *files, *order, *out, cutoff, stdout)
	default:
		err = queueCommand(ctx, command, queueSettings{
			queue: *queue, settings: invoice.Settings{ProxyURL: *proxy, Endpoint: *endpoint},
			order: *order, template: *template, language: *language, sendTo: *sendTo,
			body: *body, out: *out, once: *once, files: files,
		}, stdout, stderr)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// missingFlags names what a command needs but didn't get.
func missingFlags(command, queue, bucket, order, template, language, body, out, before string) string {
	switch {
	case (command == "request" || command == "send" || command == "work") && queue == "":
		return "--queue (or INVOICES_QUEUE_URL)"
	case (command == "fetch" || command == "list" || command == "cleanup") && bucket == "":
		return "--bucket (or INVOICES_BUCKET)"
	case command == "request" && (order == "" || template == "" || language == ""):
		return "--order, --template and --language"
	case command == "send" && body == "":
		return "--body"
	case command == "work" && out == "" && bucket == "":
		return "--out or --bucket"
	case command == "fetch" && (order == "" || out == ""):
		return "--order and --out"
	case command == "cleanup" && before == "":
		return "--before"
	}
	return ""
}

type queueSettings struct {
	queue                             string
	settings                          invoice.Settings
	order, template, language, sendTo string
	body, out                         string
	once                              bool
	files                             *invoice.Files
}

// queueCommand carries out request, send and work, which use the queue.
func queueCommand(ctx context.Context, command string, q queueSettings, stdout, stderr io.Writer) error {
	client, err := invoice.NewClient(ctx, q.settings)
	if err != nil {
		return err
	}
	switch command {
	case "request":
		job := invoice.NewJob(q.order, q.template, q.language, q.sendTo, time.Now().UTC())
		data, err := json.Marshal(job)
		if err == nil {
			_, err = invoice.Check(data)
		}
		if err != nil {
			return err
		}
		q.body = string(data)
		fallthrough
	case "send":
		id, err := invoice.Send(ctx, client, q.queue, q.body)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(stdout, "sent %s\n", id)
	case "work":
		w := &invoice.Worker{Client: client, QueueURL: q.queue, OutDir: q.out, Files: q.files, Log: slog.New(slog.NewJSONHandler(stderr, nil))}
		if q.out != "" {
			w.Files = nil
		}
		if err := w.Run(ctx, q.once); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	return nil
}

// bucketCommand carries out fetch, list and cleanup, which use the bucket.
func bucketCommand(ctx context.Context, command string, files invoice.Files, order, out string, before time.Time, stdout io.Writer) error {
	switch command {
	case "fetch":
		pdf, err := files.Fetch(ctx, order)
		if err != nil {
			return err
		}
		return os.WriteFile(out, pdf, 0o600) //nolint:gosec // the user names the file to write
	case "list":
		list, err := files.List(ctx)
		if err != nil {
			return err
		}
		for _, f := range list {
			_, _ = fmt.Fprintf(stdout, "%s\t%d\t%s\n", f.Key, f.Size, f.Modified.UTC().Format(time.RFC3339))
		}
	case "cleanup":
		deleted, err := files.Cleanup(ctx, before)
		if err != nil {
			return err
		}
		for _, key := range deleted {
			_, _ = fmt.Fprintf(stdout, "deleted %s\n", key)
		}
	}
	return nil
}
