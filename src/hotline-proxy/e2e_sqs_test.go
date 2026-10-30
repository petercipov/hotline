//go:build e2e

package proxy_test

import (
	"context"
	"crypto/md5" //nolint:gosec // SQS's own message checksum
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
)

const invoicesQueueURL = "https://sqs.eu-west-1.amazonaws.com/192325781350/invoices"

// fakeSQS answers SendMessage, ReceiveMessage and DeleteMessage the way SQS
// does over its JSON protocol, over TLS as localhost.
type fakeSQS struct {
	server   *httptest.Server
	mu       sync.Mutex
	calls    []*http.Request
	messages []sqsMessage
	next     int
}

type sqsMessage struct {
	id, body, receipt string
	inFlight          bool
}

func newFakeSQS(cert tls.Certificate) *fakeSQS {
	f := &fakeSQS{}
	f.server = httptest.NewUnstartedServer(f)
	f.server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	f.server.StartTLS()
	return f
}

func (f *fakeSQS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var in map[string]any
	_ = json.NewDecoder(r.Body).Decode(&in)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Clone(context.Background()))
	var out any
	switch r.Header.Get("X-Amz-Target") {
	case "AmazonSQS.SendMessage":
		f.next++
		body, _ := in["MessageBody"].(string)
		m := sqsMessage{id: fmt.Sprintf("m-%d", f.next), body: body, receipt: fmt.Sprintf("r-%d", f.next)}
		f.messages = append(f.messages, m)
		out = map[string]any{"MessageId": m.id, "MD5OfMessageBody": md5Hex(m.body)}
	case "AmazonSQS.ReceiveMessage":
		var got []map[string]any
		for i := range f.messages {
			m := &f.messages[i]
			if !m.inFlight {
				m.inFlight = true
				got = append(got, map[string]any{"MessageId": m.id, "ReceiptHandle": m.receipt, "MD5OfBody": md5Hex(m.body), "Body": m.body})
			}
		}
		// Like SQS, an empty receive has no Messages at all.
		reply := map[string]any{}
		if len(got) > 0 {
			reply["Messages"] = got
		}
		out = reply
	case "AmazonSQS.DeleteMessage":
		f.messages = slices.DeleteFunc(f.messages, func(m sqsMessage) bool { return m.receipt == in["ReceiptHandle"] })
		out = map[string]any{}
	default:
		w.WriteHeader(http.StatusBadRequest)
		out = map[string]any{"__type": "com.amazonaws.sqs#InvalidAction", "message": "unknown action"}
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.0")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		panic(err)
	}
}

func (f *fakeSQS) waiting() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.messages)
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // SQS's own message checksum
	return hex.EncodeToString(sum[:])
}

// exampleApp builds the example program once per run.
type exampleApp struct {
	dir    string
	once   sync.Once
	binary string
	err    error
}

func (e *exampleApp) build() (string, error) {
	e.once.Do(func() {
		e.binary = filepath.Join(e.dir, "aws-sqs-invoices")
		build := exec.CommandContext(context.Background(), "go", "build", "-o", e.binary, "hotline-example/aws-sqs-invoices") //nolint:gosec // builds the example app
		if out, err := build.CombinedOutput(); err != nil {
			e.err = fmt.Errorf("%w: building the example app: %s", err, out)
		}
	})
	return e.binary, e.err
}

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
// address), and the spec and overlay read from where they are in the repo.
func exampleConfig(proxyAddr, otlpEndpoint string) (string, error) {
	dir, err := filepath.Abs(filepath.Join("testdata", "aws"))
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "hotline-proxy.yaml")) //nolint:gosec // the test's own file
	if err != nil {
		return "", err
	}
	config := string(data)
	for _, change := range [][2]string{
		{`":8080"`, fmt.Sprintf("%q", proxyAddr)},
		{`":8081"`, `"127.0.0.1:0"`},
		{"localhost:4317", otlpEndpoint},
		{"[sqs.eu-west-1.amazonaws.com]", "[localhost]\n    allowInternalIPs: true"},
		{"../../../../specs/", filepath.Join(dir, "..", "..", "..", "..", "specs") + "/"},
		{"./invoices.overlay.yaml", filepath.Join(dir, "invoices.overlay.yaml")},
	} {
		if strings.Count(config, change[0]) != 1 {
			return "", failf("the example config no longer has %s once", change[0])
		}
		config = strings.Replace(config, change[0], change[1], 1)
	}
	return config, nil
}
