//go:build e2e

package proxy_test

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

const invoicesQueueURL = "https://sqs.eu-west-1.amazonaws.com/192325781350/invoices"

func sqsScenario(sc *godog.ScenarioContext, p *program, app *exampleApp) {
	var sqs *fakeSQS
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
	sc.Step(`^the hotline-proxy program runs with the shop's hotline-proxy.yaml$`, func() error {
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
			"HOTLINE_PROXY=http://" + p.proxyAddr,
			"SQS_ENDPOINT=https://localhost:" + port(sqs.server),
			"HOME=" + p.dir,
		}
		if output, err := cmd.CombinedOutput(); err != nil {
			return failf("the example app %v failed: %v\n%s\nhotline-proxy said:\n%s", args, err, output, p.stderr.String())
		}
		return nil
	}
	sc.Step(`^checkout sends the message (.+)$`, func(body string) error {
		return example("checkout", "send", "--body", body)
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
	sc.Step(`^the receiver got one SendMessage record from the integration checkout, with no results$`, func() error {
		records, err := p.records()
		if err != nil {
			return err
		}
		if len(records) != 1 || records[0]["url.template"] != "/#X-Amz-Target=AmazonSQS.SendMessage" || records[0]["hotline.integration.id"] != "checkout" {
			return failf("want one SendMessage record from checkout, got %v", records)
		}
		for _, key := range []string{"hotline.schema.request.errors", "hotline.schema.response.errors", "hotline.schema.skipped"} {
			if v, found := records[0][key]; found {
				return failf("the record has %s %v", key, v)
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
// addresses changed: the local ports, the invoices queue on the fake SQS on
// localhost (an internal address), and the specs, overlays and key read
// from where they are in the repo. The other AWS hosts stay as they are, so
// only the invoices queue can be reached.
func exampleConfig(proxyAddr, otlpEndpoint string) (string, error) {
	config, err := shopConfig(
		[2]string{`":8080"`, fmt.Sprintf("%q", proxyAddr)},
		[2]string{"localhost:4317", otlpEndpoint},
	)
	if err != nil {
		return "", err
	}
	// The host line of the invoices queue's api; the mail queue has the same host.
	invoicesQueueHost := regexp.MustCompile(`(?m)^(  aws-sqs-invoices:.*\n    hosts: )\[sqs\.eu-west-1\.amazonaws\.com\]`)
	if !invoicesQueueHost.MatchString(config) {
		return "", failf("the example config no longer names the invoices queue's host")
	}
	return invoicesQueueHost.ReplaceAllString(config, "${1}[localhost]\n    allowInternalIPs: true"), nil
}
