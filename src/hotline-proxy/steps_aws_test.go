package proxy_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

const (
	sqsHost       = "sqs.eu-west-1.amazonaws.com"
	invoicesQueue = "https://sqs.eu-west-1.amazonaws.com/123456789012/invoices"
)

func awsSteps(sc *godog.ScenarioContext, w *world) {
	sc.Step(`^the SQS spec$`, func() error {
		return w.sqsAPI("talon-shop-a", "aws-sqs", "")
	})
	sc.Step(`^the SQS spec with the invoices overlay$`, func() error {
		overlay, err := os.ReadFile(filepath.Join("testdata", "aws", "invoices.overlay.yaml"))
		w.writeFile("specs/invoices.overlay.yaml", string(overlay))
		if err != nil {
			return err
		}
		return w.sqsAPI("talon-shop-a", "aws-sqs-invoices", "    overlays: [./specs/invoices.overlay.yaml]\n")
	})
	sc.Step(`^the integration (\S+) with the plain SQS spec$`, func(id string) error {
		return w.sqsAPI(id, "aws-sqs", "")
	})
	sc.Step(`^the SQS api with a spec whose route is (\S+) and (no x-hotline-action-header|x-hotline-action-header: \S+)$`, func(route, mark string) error {
		header := ""
		if strings.HasPrefix(mark, "x-hotline-action-header:") {
			header = mark + "\n"
		}
		spec := "openapi: 3.1.0\ninfo: { title: SQS, version: '1' }\n" + header +
			"paths:\n  " + route + ":\n    post:\n      responses:\n        '200': { description: done }\n"
		w.addIntegration(integration("talon-shop-a", w.specAPI(sqsHost, spec, "")))
		return nil
	})
	sc.Step(`^SQS answers (\d+)(?: with "([^"]*)")? and the json body (.*)$`, func(status int, headers, body string) error {
		w.sqsAnswers(status, headers, body)
		return nil
	})
	sc.Step(`^SQS answers 200 with the json body (.*)$`, func(body string) error {
		w.sqsAnswers(http.StatusOK, "", body)
		return nil
	})
	sc.Step(`^SQS answers 200 with one received message (.*)$`, func(message string) error {
		reply, err := json.Marshal(map[string]any{"Messages": []any{map[string]any{
			"MessageId": "m-1", "ReceiptHandle": "r-1", "MD5OfBody": "x", "Body": message,
		}}})
		w.sqsAnswers(http.StatusOK, "", string(reply))
		return err
	})
	sc.Step(`^the app sends (\w+) with the json body (.*)$`, func(action, body string) error {
		return w.sqsCall(w.appID, []string{"X-Amz-Target: AmazonSQS." + action}, body)
	})
	sc.Step(`^the app sends POST / (without X-Amz-Target|with "([^"]*)") with the json body (.*)$`, func(_, header, body string) error {
		var headers []string
		if header != "" {
			headers = append(headers, header)
		}
		return w.sqsCall(w.appID, headers, body)
	})
	sc.Step(`^(?:the app|(\S+)) sends SendMessage to (the invoices queue|\S+) with the message (.*)$`, func(id, queue, message string) error {
		if id == "" {
			id = w.appID
		}
		if queue == "the invoices queue" {
			queue = invoicesQueue
		}
		body, err := json.Marshal(map[string]string{"QueueUrl": queue, "MessageBody": message})
		if err != nil {
			return err
		}
		return w.sqsCall(id, []string{"X-Amz-Target: AmazonSQS.SendMessage"}, string(body))
	})
	sc.Step(`^no result holds (\S+)$`, func(value string) error {
		record, err := w.record()
		if err != nil {
			return err
		}
		all := fmt.Sprint(record["hotline.schema.request.errors"], record["hotline.schema.response.errors"])
		if strings.Contains(all, value) {
			return failf("the results %s hold a value from the call", all)
		}
		return nil
	})
	sc.Step(`^the record keeps the reply headers (\S+) and (\S+)$`, func(first, second string) error {
		record, err := w.record()
		if err != nil {
			return err
		}
		return hasAll(record, "http.response.header."+first, "http.response.header."+second)
	})
}

// sqsAPI adds an api for SQS with the stable spec from the specs folder,
// and the integration id that uses it.
func (w *world) sqsAPI(id, name, extra string) error {
	spec, err := os.ReadFile(filepath.Join("..", "..", "specs", "aws", "sqs", "2012-11-05.openapi.yaml"))
	w.writeFile("specs/aws-sqs.openapi.yaml", string(spec))
	line := w.api(name, "    hosts: ["+sqsHost+"]\n    spec: ./specs/aws-sqs.openapi.yaml\n"+extra)
	w.addIntegration(integration(id, line))
	return err
}

// sqsAnswers makes the fake SQS answer every call the way SQS does.
func (w *world) sqsAnswers(status int, headers, body string) {
	w.vendor(sqsHost).setAnswer(func(rw http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		rw.Header().Set("Content-Type", "application/x-amz-json-1.0")
		for _, h := range splitHeaders(headers) {
			name, value, _ := strings.Cut(h, ": ")
			rw.Header().Set(name, value)
		}
		rw.WriteHeader(status)
		_, _ = rw.Write([]byte(body))
	})
}

// sqsCall sends an SQS call through the forward proxy, the way the AWS SDK
// does after the app's client turned https into http.
func (w *world) sqsCall(id string, headers []string, body string) error {
	lines := []string{
		"Content-Type: application/x-amz-json-1.0",
		"Authorization: AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261030/eu-west-1/sqs/aws4_request, SignedHeaders=host;x-amz-date;x-amz-target, Signature=0",
		"X-Amz-Date: 20261030T193000Z",
	}
	if id != "" {
		lines = append([]string{basic(id + ":x")}, lines...)
	}
	return w.send(request{
		method:  http.MethodPost,
		target:  "http://" + sqsHost + "/",
		headers: append(lines, headers...),
		body:    strings.NewReader(body),
		length:  len(body),
	})
}
