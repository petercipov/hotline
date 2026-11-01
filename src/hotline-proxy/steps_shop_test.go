package proxy_test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"go.yaml.in/yaml/v3"
)

// shopSteps drive the proxy with the apis of the shop example's config,
// testdata/aws/hotline-proxy.yaml: each its stable spec and its overlay.
func shopSteps(sc *godog.ScenarioContext, w *world) {
	var host string
	sc.Step(`^the shop api (\S+)$`, func(name string) error {
		var err error
		host, err = w.exampleAPI(name)
		return err
	})
	sc.Step(`^AWS answers (\d+) with the json body (.*)$`, func(status int, body string) error {
		w.vendor(host).setAnswer(func(rw http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			rw.Header().Set("Content-Type", r.Header.Get("Content-Type"))
			rw.WriteHeader(status)
			_, _ = rw.Write([]byte(body))
		})
		return nil
	})
	sc.Step(`^the app calls (\S+) with the json body (.*)$`, func(target, body string) error {
		return w.awsCall(host, target, body)
	})
	sc.Step(`^the app puts a Kinesis record holding (.*)$`, func(data string) error {
		body, err := json.Marshal(map[string]string{
			"StreamName": "shop-events", "PartitionKey": "A-1001",
			"Data": base64.StdEncoding.EncodeToString([]byte(data)),
		})
		if err != nil {
			return err
		}
		return w.awsCall(host, "Kinesis_20131202.PutRecord", string(body))
	})
}

// exampleAPI adds the named api of the shop example's config, with its spec and
// overlays, used by the app's integration, and gives its host.
func (w *world) exampleAPI(name string) (string, error) {
	dir := filepath.Join("testdata", "aws")
	data, err := os.ReadFile(filepath.Join(dir, "hotline-proxy.yaml")) //nolint:gosec // the test's own file
	if err != nil {
		return "", err
	}
	var config struct {
		APIs map[string]struct {
			Hosts    []string `yaml:"hosts"`
			Spec     string   `yaml:"spec"`
			Overlays []string `yaml:"overlays"`
		} `yaml:"apis"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", err
	}
	api, ok := config.APIs[name]
	if !ok {
		return "", failf("the shop config has no api %s", name)
	}
	copyFile := func(from string) (string, error) {
		content, err := os.ReadFile(filepath.Join(dir, from)) //nolint:gosec // files the test's config names
		to := "specs/" + strings.NewReplacer("/", "-", "..-", "").Replace(from)
		w.writeFile(to, string(content))
		return "./" + to, err
	}
	spec, err := copyFile(api.Spec)
	if err != nil {
		return "", err
	}
	var overlays []string
	for _, o := range api.Overlays {
		path, err := copyFile(o)
		if err != nil {
			return "", err
		}
		overlays = append(overlays, path)
	}
	line := w.api(name, "    hosts: ["+api.Hosts[0]+"]\n    spec: "+spec+"\n    overlays: ["+strings.Join(overlays, ", ")+"]\n")
	w.addIntegration(integration(w.appID, line))
	return api.Hosts[0], nil
}

// awsCall sends one call on AWS's JSON protocol, as the SDK signs it, to
// the host, naming the action in X-Amz-Target.
func (w *world) awsCall(host, target, body string) error {
	// DynamoDB, SQS and Step Functions speak AWS JSON 1.0, the others 1.1.
	version := "1.1"
	for _, prefix := range []string{"DynamoDB_", "AmazonSQS.", "AWSStepFunctions."} {
		if strings.HasPrefix(target, prefix) {
			version = "1.0"
		}
	}
	lines := []string{
		"Content-Type: application/x-amz-json-" + version,
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261101/eu-west-1/x/aws4_request, SignedHeaders=host;x-amz-date;x-amz-target, Signature=0",
		"X-Amz-Date: 20261101T100000Z",
		"X-Amz-Target: " + target,
	}
	if w.appID != "" {
		lines = append([]string{basic(w.appID + ":x")}, lines...)
	}
	return w.send(request{
		method:  http.MethodPost,
		target:  "http://" + host + "/",
		headers: lines,
		body:    strings.NewReader(body),
		length:  len(body),
	})
}
