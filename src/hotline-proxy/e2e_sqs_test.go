//go:build e2e

package proxy_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

const invoicesQueueURL = "https://sqs.eu-west-1.amazonaws.com/192325781350/invoices"

func sqsScenario(sc *godog.ScenarioContext, p *program, app *exampleApp) {
	var (
		sqs *fakeSQS
		out string
	)
	sc.After(func(ctx context.Context, _ *godog.Scenario, err error) (context.Context, error) {
		if sqs != nil {
			sqs.server.Close()
			sqs = nil
		}
		return ctx, err
	})
	sc.Step(`^a fake SQS$`, func() error {
		sqs = newFakeSQS(p.issuer.leaf("localhost", time.Now().Add(24*time.Hour), false))
		return nil
	})
	sc.Step(`^the hotline-proxy program runs with the invoices hotline-proxy.yaml$`, func() error {
		p.pickPorts()
		config, err := exampleConfig(p.proxyAddr, p.receiver.endpoint)
		if err != nil {
			return err
		}
		p.writeConfig(config)
		return p.launch()
	})
	example := func(args ...string) error {
		binary, err := app.build()
		if err != nil {
			return err
		}
		cmd := exec.CommandContext(context.Background(), binary, args...) //nolint:gosec // the example app
		cmd.Dir = p.dir
		cmd.Env = []string{
			"AWS_REGION=eu-west-1",
			"AWS_ACCESS_KEY_ID=AKIDEXAMPLE",
			"AWS_SECRET_ACCESS_KEY=secret",
			"AWS_EC2_METADATA_DISABLED=true",
			"INVOICES_QUEUE_URL=" + invoicesQueueURL,
			"HOTLINE_PROXY=http://invoices:x@" + p.proxyAddr,
			"SQS_ENDPOINT=https://localhost:" + port(sqs.server),
			"HOME=" + p.dir,
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			return failf("the example app %v failed: %v\n%s\nhotline-proxy said:\n%s", args, err, output, p.stderr.String())
		}
		return nil
	}
	sc.Step(`^the example app requests the invoice for order (\S+)$`, func(order string) error {
		return example("request", "--order", order, "--template", "invoice-standard-v2", "--language", "en")
	})
	sc.Step(`^the example app sends the message (.+)$`, func(body string) error {
		return example("send", "--body", body)
	})
	sc.Step(`^the example app works through the queue once$`, func() error {
		out = filepath.Join(p.dir, "invoices")
		if err := os.MkdirAll(out, 0o750); err != nil {
			return err
		}
		return example("work", "--out", out, "--once")
	})
	sc.Step(`^the worker wrote (\S+)$`, func(name string) error {
		data, err := os.ReadFile(filepath.Join(out, name)) //nolint:gosec // the test's own folder
		if err != nil || !strings.HasPrefix(string(data), "%PDF-") {
			return failf("the worker wrote no PDF %s: %v", name, err)
		}
		return nil
	})
	sc.Step(`^the fake SQS has no messages left$`, func() error {
		if n := sqs.waiting(); n != 0 {
			return failf("the fake SQS still has %d messages", n)
		}
		return nil
	})
	sc.Step(`^the fake SQS has (\d+) messages? waiting$`, func(n int) error {
		if got := sqs.waiting(); got != n {
			return failf("the fake SQS has %d messages, want %d", got, n)
		}
		return nil
	})
	sc.Step(`^the fake SQS got every call signed by the SDK, over https, without Proxy-Authorization$`, func() error {
		sqs.mu.Lock()
		defer sqs.mu.Unlock()
		if len(sqs.calls) == 0 {
			return failf("the fake SQS got no calls")
		}
		for _, r := range sqs.calls {
			if r.TLS == nil || !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") ||
				r.Header.Get("Proxy-Authorization") != "" || r.Host != "localhost:"+port(sqs.server) {
				return failf("the fake SQS got %s with Host %s, TLS %v and headers %v", r.Header.Get("X-Amz-Target"), r.Host, r.TLS != nil, r.Header)
			}
		}
		return nil
	})
	sc.Step(`^the receiver got records for SendMessage, ReceiveMessage and DeleteMessage, each with its url.template and no results$`, func() error {
		records, err := p.records()
		if err != nil {
			return err
		}
		var templates []string
		for _, r := range records {
			templates = append(templates, fmt.Sprint(r["url.template"]))
			for _, key := range []string{"hotline.schema.request.errors", "hotline.schema.response.errors", "hotline.schema.skipped"} {
				if v, found := r[key]; found {
					return failf("the record for %v has %s %v", r["url.template"], key, v)
				}
			}
		}
		for _, action := range []string{"SendMessage", "ReceiveMessage", "DeleteMessage"} {
			if !slices.Contains(templates, "/#X-Amz-Target=AmazonSQS."+action) {
				return failf("no record for %s; the records have the templates %v", action, templates)
			}
		}
		return nil
	})
	sc.Step(`^the receiver got a SendMessage record with the request results (.+)$`, func(want string) error {
		records, err := p.records()
		if err != nil {
			return err
		}
		if len(records) != 1 || records[0]["url.template"] != "/#X-Amz-Target=AmazonSQS.SendMessage" {
			return failf("want one SendMessage record, got %v", records)
		}
		var got []string
		list, _ := records[0]["hotline.schema.request.errors"].([]any)
		for _, item := range list {
			r, _ := item.(map[string]any)
			parts := []string{fmt.Sprint(r["part"]), fmt.Sprint(r["place"]), fmt.Sprint(r["rule"])}
			if e, found := r["expected"]; found {
				parts = append(parts, fmt.Sprint(e))
			}
			got = append(got, strings.Join(parts, " "))
		}
		if strings.Join(got, "; ") != want {
			return failf("the request results are %q, want %q", strings.Join(got, "; "), want)
		}
		return nil
	})
}

// records are every request record the receiver got, once the program
// stopped.
func (p *program) records() ([]map[string]any, error) {
	if err := p.stop(); err != nil {
		return nil, err
	}
	received, _ := p.receiver.got()
	var out []map[string]any
	for _, r := range received {
		if r.eventName == "hotline.request" {
			out = append(out, r.attrs)
		}
	}
	return out, nil
}

// exampleConfig is testdata/aws/hotline-proxy.yaml with only the
// addresses changed: the local ports, the fake SQS on localhost (an internal
// address), and the spec and overlays read from where they are in the repo.
func exampleConfig(proxyAddr, otlpEndpoint string) (string, error) {
	return invoicesConfig(
		[2]string{`":8080"`, fmt.Sprintf("%q", proxyAddr)},
		[2]string{"localhost:4317", otlpEndpoint},
		[2]string{"[sqs.eu-west-1.amazonaws.com]", "[localhost]\n    allowInternalIPs: true"},
	)
}
