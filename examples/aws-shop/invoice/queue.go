package invoice

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// Settings say how the SQS client reaches SQS.
type Settings struct {
	// ProxyURL is Hotline's forward proxy with the integration ID as the
	// user name, such as http://invoices:x@hotline:8080. Empty: no proxy.
	ProxyURL string
	// Endpoint replaces SQS's own address, for a local SQS. Empty: AWS.
	Endpoint string
}

// toHTTP turns https into http after the SDK has signed the call, so it goes
// to the forward proxy as a plain request Hotline can read. The scheme is not
// part of the signature. Hotline sends it on to AWS over https.
type toHTTP struct{ next http.RoundTripper }

func (t toHTTP) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.URL.Scheme = "http"
	return t.next.RoundTrip(r)
}

// NewClient makes an SQS client. Region and credentials come from the usual
// AWS settings (environment, shared files, the pod's role); opts can add to
// them. Only this client goes through Hotline, so the SDK's own credential
// lookups never reach the proxy.
func NewClient(ctx context.Context, s Settings, opts ...func(*config.LoadOptions) error) (*sqs.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	client, err := s.HTTPClient()
	if err != nil {
		return nil, err
	}
	return sqs.NewFromConfig(cfg, func(o *sqs.Options) {
		if s.Endpoint != "" {
			o.BaseEndpoint = aws.String(s.Endpoint)
		}
		if client != nil {
			o.HTTPClient = client
		}
	}), nil
}

// HTTPClient is the HTTP client that reaches Hotline, nil without a proxy.
func (s Settings) HTTPClient() (*http.Client, error) {
	if s.ProxyURL == "" {
		return nil, nil //nolint:nilnil // no proxy: the SDK's own client
	}
	proxy, err := url.Parse(s.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("the proxy address can't be read: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Go adds Proxy-Authorization from the user name here, after signing,
	// so the signature stays valid when Hotline removes it.
	transport.Proxy = http.ProxyURL(proxy)
	return &http.Client{Transport: toHTTP{transport}}, nil
}

// Send puts one message on the queue and returns its SQS message ID.
func Send(ctx context.Context, c *sqs.Client, queueURL, body string) (string, error) {
	out, err := c.SendMessage(ctx, &sqs.SendMessageInput{QueueUrl: &queueURL, MessageBody: &body})
	if err != nil {
		return "", err
	}
	return aws.ToString(out.MessageId), nil
}

// Worker receives invoice jobs and writes each invoice to OutDir, or
// stores it in Files when set.
type Worker struct {
	Client   *sqs.Client
	QueueURL string
	OutDir   string
	Files    *Files
	Log      *slog.Logger
	// DefaultTemplate renders a job that asks for the template "default".
	DefaultTemplate string
	// Made, when set, hears of each invoice once it is kept, before the
	// message is deleted. When it fails, the message stays on the queue to
	// be tried again.
	Made func(ctx context.Context, job Job, pdf []byte) error
	// PaymentText, when set, gives the payment text a bank app reads from
	// a QR code, printed on the invoice. When it fails, the message stays
	// on the queue to be tried again.
	PaymentText func(ctx context.Context, job Job) (string, error)

	seen map[string]bool
}

// Run receives jobs until ctx ends. With once set, it stops as soon as the
// queue has nothing more to give, without waiting for new messages.
func (w *Worker) Run(ctx context.Context, once bool) error {
	wait := int32(20) // long polling: SQS holds the call up to 20 seconds
	if once {
		wait = 0
	}
	for ctx.Err() == nil {
		out, err := w.Client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            &w.QueueURL,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     wait,
		})
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		if once && len(out.Messages) == 0 {
			return nil
		}
		for _, m := range out.Messages {
			if err := w.handle(ctx, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// handle does one job. A message the worker can't use stays on the queue:
// after the queue's maxReceiveCount, SQS moves it to the dead letter queue.
func (w *Worker) handle(ctx context.Context, m types.Message) error {
	job, err := Check([]byte(aws.ToString(m.Body)))
	if err != nil {
		w.Log.Warn("message refused", "messageId", aws.ToString(m.MessageId), "reason", err.Error())
		return nil
	}
	if w.seen[job.EventID] {
		w.Log.Info("event already handled", "eventId", job.EventID)
		return w.delete(ctx, m)
	}
	if job.TemplateID == "default" && w.DefaultTemplate != "" {
		job.TemplateID = w.DefaultTemplate
	}
	var extra []string
	if w.PaymentText != nil {
		text, err := w.PaymentText(ctx, job)
		if err != nil {
			return err
		}
		if text != "" {
			extra = append(extra, "", "Pay: "+text)
		}
	}
	pdf, err := Render(job, extra...)
	if err != nil {
		w.Log.Warn("message refused", "messageId", aws.ToString(m.MessageId), "eventId", job.EventID, "reason", err.Error())
		return nil
	}
	file, err := w.keep(ctx, job.OrderID, pdf)
	if err != nil {
		return err
	}
	w.Log.Info("invoice written", "eventId", job.EventID, "file", file)
	if w.Made != nil {
		if err := w.Made(ctx, job, pdf); err != nil {
			return err
		}
	}
	if w.seen == nil {
		w.seen = map[string]bool{}
	}
	w.seen[job.EventID] = true
	return w.delete(ctx, m)
}

// keep stores the PDF in the bucket, or writes it to the folder, and says
// where it went.
func (w *Worker) keep(ctx context.Context, orderID string, pdf []byte) (string, error) {
	if w.Files != nil {
		return w.Files.Bucket + "/" + Key(orderID), w.Files.Store(ctx, orderID, pdf)
	}
	file := filepath.Join(w.OutDir, "invoice-"+orderID+".pdf")
	if err := os.MkdirAll(w.OutDir, 0o750); err != nil {
		return "", fmt.Errorf("write %s: %w", file, err)
	}
	return file, os.WriteFile(file, pdf, 0o600)
}

func (w *Worker) delete(ctx context.Context, m types.Message) error {
	_, err := w.Client.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: &w.QueueURL, ReceiptHandle: m.ReceiptHandle})
	return err
}
