package shop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// readyEvent is EventBridge's event as the rule puts it on the mail queue.
type readyEvent struct {
	DetailType string       `json:"detail-type"` //nolint:tagliatelle // EventBridge's own name
	Detail     invoiceReady `json:"detail"`
}

var errNotReady = errors.New("not an InvoiceReady event")

// Mail is the mailer: it takes InvoiceReady events off the mail queue,
// fetches each PDF from the bucket and writes the email, with the PDF
// attached, to the folder out, as A-1001.eml. Sending it with SES comes in
// a later step. With once set, it stops when the queue is empty.
func (a App) Mail(ctx context.Context, once bool, out string) error {
	wait := int32(20)
	if once {
		wait = 0
	}
	for ctx.Err() == nil {
		got, err := a.SQS.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl: aws.String(a.Names.MailQueue), MaxNumberOfMessages: 10, WaitTimeSeconds: wait,
		})
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil
		}
		if err != nil {
			return err
		}
		if once && len(got.Messages) == 0 {
			return nil
		}
		for _, m := range got.Messages {
			if err := a.mailOne(ctx, m, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a App) mailOne(ctx context.Context, m sqstypes.Message, out string) error {
	var e readyEvent
	err := json.Unmarshal([]byte(aws.ToString(m.Body)), &e)
	if err == nil && (e.DetailType != "InvoiceReady" || e.Detail.OrderID == "") {
		err = errNotReady
	}
	if err != nil {
		// It stays on the queue; after maxReceiveCount SQS moves it aside.
		a.Log.Warn("message refused", "messageId", aws.ToString(m.MessageId), "reason", err.Error())
		return nil
	}
	pdf, err := a.Files.Fetch(ctx, e.Detail.OrderID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}
	file := filepath.Join(out, e.Detail.OrderID+".eml")
	if err := os.WriteFile(file, email(e.Detail, pdf), 0o600); err != nil {
		return err
	}
	if _, err := a.SQS.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(a.Names.MailQueue), ReceiptHandle: m.ReceiptHandle}); err != nil {
		return err
	}
	return a.note(ctx, "invoice "+e.Detail.OrderID+" mailed", "orderId", e.Detail.OrderID, "file", file)
}

// email writes a plain MIME email with the PDF attached.
func email(d invoiceReady, pdf []byte) []byte {
	to := d.SendTo
	if to == "" {
		to = "customer@example.com"
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "To: %s\r\nFrom: shop@example.com\r\nSubject: Your invoice for order %s\r\n", to, d.OrderID)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=invoice\r\n\r\n")
	fmt.Fprintf(&b, "--invoice\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThank you for your order %s. Your invoice is attached.\r\n", d.OrderID)
	fmt.Fprintf(&b, "--invoice\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=%s\r\nContent-Transfer-Encoding: base64\r\n\r\n", d.File)
	b.WriteString(base64.StdEncoding.EncodeToString(pdf))
	b.WriteString("\r\n--invoice--\r\n")
	return b.Bytes()
}
