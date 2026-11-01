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
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

// readyEvent is EventBridge's event as the rule puts it on the mail queue.
type readyEvent struct {
	DetailType string       `json:"detail-type"` //nolint:tagliatelle // EventBridge's own name
	Detail     invoiceReady `json:"detail"`
}

var errNotReady = errors.New("not an InvoiceReady event")

// ErrNoMailWay means the mailer has neither a sender for SES nor a folder.
var ErrNoMailWay = errors.New("the mailer needs MAIL_FROM, an SES sender, or --out, a folder")

// Mail is the mailer: it takes InvoiceReady events off the mail queue,
// fetches each PDF from the bucket and sends the email, with the PDF
// attached, with SES from the shop's sender. With out set, it writes the
// email to that folder instead, as A-1001.eml, and sends nothing. With once
// set, it stops when the queue is empty.
func (a App) Mail(ctx context.Context, once bool, out string) error {
	if out == "" && a.Names.MailFrom == "" {
		return ErrNoMailWay
	}
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
	from := a.Names.MailFrom
	if from == "" {
		from = "shop@example.com"
	}
	mail := email(e.Detail, from, pdf)
	where, err := a.deliver(ctx, e.Detail, from, mail, out)
	if err != nil {
		return err
	}
	if _, err := a.SQS.DeleteMessage(ctx, &sqs.DeleteMessageInput{QueueUrl: aws.String(a.Names.MailQueue), ReceiptHandle: m.ReceiptHandle}); err != nil {
		return err
	}
	return a.note(ctx, "invoice "+e.Detail.OrderID+" mailed", "orderId", e.Detail.OrderID, "sent", where)
}

// deliver writes the email to the folder out, when set, or else sends it
// with SES. It says where the email went.
func (a App) deliver(ctx context.Context, d invoiceReady, from string, mail []byte, out string) (string, error) {
	if out != "" {
		if err := os.MkdirAll(out, 0o750); err != nil {
			return "", err
		}
		file := filepath.Join(out, d.OrderID+".eml")
		return file, os.WriteFile(file, mail, 0o600)
	}
	sent, err := a.SES.SendEmail(ctx, &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(from),
		Destination:      &sestypes.Destination{ToAddresses: []string{recipient(d)}},
		Content:          &sestypes.EmailContent{Raw: &sestypes.RawMessage{Data: mail}},
	})
	if err != nil {
		return "", fmt.Errorf("send the email: %w", err)
	}
	return "SES message " + aws.ToString(sent.MessageId), nil
}

// recipient is where the invoice goes: the customer's address, or a made
// up one for an order without it.
func recipient(d invoiceReady) string {
	if d.SendTo == "" {
		return "customer@example.com"
	}
	return d.SendTo
}

// email writes a plain MIME email with the PDF attached.
func email(d invoiceReady, from string, pdf []byte) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "To: %s\r\nFrom: %s\r\nSubject: Your invoice for order %s\r\n", recipient(d), from, d.OrderID)
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=invoice\r\n\r\n")
	fmt.Fprintf(&b, "--invoice\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThank you for your order %s. Your invoice is attached.\r\n", d.OrderID)
	fmt.Fprintf(&b, "--invoice\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=%s\r\nContent-Transfer-Encoding: base64\r\n\r\n", d.File)
	b.WriteString(base64.StdEncoding.EncodeToString(pdf))
	b.WriteString("\r\n--invoice--\r\n")
	return b.Bytes()
}
