package shop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"hotline-example/aws-shop/invoice"
)

// Payment is a customer paying for an order.
type Payment struct {
	OrderID  string
	Language string
	Template string // "default" takes the shop's setting
	SendTo   string // where to mail the invoice; may be empty
}

// ErrNoPaymentKey means the payment secret is empty.
var ErrNoPaymentKey = errors.New("the payment key is empty")

// Pay is checkout taking a payment: it reads the payment key (the card
// charge itself is left out), saves the order, writes order-paid to the
// stream, and puts the invoice job on the invoices queue. It gives the
// job's SQS message ID.
func (a App) Pay(ctx context.Context, p Payment, now time.Time) (string, error) {
	secret, err := a.Secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(a.Names.PaymentSecret)})
	if err != nil {
		return "", fmt.Errorf("read the payment key: %w", err)
	}
	if aws.ToString(secret.SecretString) == "" {
		return "", ErrNoPaymentKey
	}
	total, currency, err := invoice.OrderTotal(p.OrderID)
	if err != nil {
		return "", err
	}
	paidAt := rfc3339(now)
	_, err = a.DynamoDB.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(a.Names.OrdersTable),
		Item: map[string]types.AttributeValue{
			"orderId":  &types.AttributeValueMemberS{Value: p.OrderID},
			"total":    &types.AttributeValueMemberN{Value: total},
			"currency": &types.AttributeValueMemberS{Value: currency},
			"language": &types.AttributeValueMemberS{Value: p.Language},
			"paidAt":   &types.AttributeValueMemberS{Value: paidAt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save the order: %w", err)
	}
	if err := a.announce(ctx, event{Type: "order-paid", OrderID: p.OrderID, At: paidAt, Total: total}); err != nil {
		return "", fmt.Errorf("announce the order: %w", err)
	}
	job := invoice.NewJob(p.OrderID, p.Template, p.Language, p.SendTo, now.UTC())
	body, err := json.Marshal(job)
	if err == nil {
		_, err = invoice.Check(body)
	}
	if err != nil {
		return "", err
	}
	id, err := invoice.Send(ctx, a.SQS, a.Names.InvoicesQueue, string(body))
	if err != nil {
		return "", fmt.Errorf("ask for the invoice: %w", err)
	}
	return id, a.note(ctx, "order "+p.OrderID+" paid", "orderId", p.OrderID, "total", total, "currency", currency)
}
