// Package shop is the shop example's four apps: checkout, invoices, mailer
// and refunds. Each app is its own Hotline integration: its calls reach the
// proxy with the app's name as the integration ID.
package shop

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"hotline-example/aws-shop/invoice"
)

// Names are the AWS resources the shop uses.
type Names struct {
	InvoicesQueue      string // SQS queue URL: fed by the order-paid topic, read by invoices
	OrderPaidTopic     string // SNS topic ARN: checkout publishes invoice jobs on it
	MailQueue          string // SQS queue URL: fed by an EventBridge rule on the bus, read by mailer
	Bucket             string // S3 bucket for the invoice PDFs
	OrdersTable        string // DynamoDB table: checkout saves orders, refunds reads them
	InvoicesTable      string // DynamoDB table: invoices saves a row per invoice
	Stream             string // Kinesis stream for shop events
	Bus                string // EventBridge bus for InvoiceReady
	PaymentSecret      string // Secrets Manager secret checkout reads
	TemplateParameter  string // SSM parameter: the default invoice template
	SigningKey         string // KMS key that signs each PDF
	CreditNoteWorkflow string // Step Functions state machine ARN refunds starts
	LogGroup           string // CloudWatch Logs group; empty: no logs sent there
	WorkerRole         string // IAM role ARN invoices takes on through STS; empty: no role
	PaymentQRFunction  string // Lambda function giving the payment text; empty: none on the invoice
	MailFrom           string // SES sender, verified in SES; empty: mailer only writes files
}

// DefaultNames are the names in Peter's AWS account, 192325781350, in
// eu-west-1.
func DefaultNames() Names {
	return Names{ //nolint:gosec // resource names, not credentials
		InvoicesQueue:      "https://sqs.eu-west-1.amazonaws.com/192325781350/invoices",
		OrderPaidTopic:     "arn:aws:sns:eu-west-1:192325781350:order-paid",
		MailQueue:          "https://sqs.eu-west-1.amazonaws.com/192325781350/mail",
		Bucket:             "shop-invoices-192325781350",
		OrdersTable:        "orders",
		InvoicesTable:      "invoices",
		Stream:             "shop-events",
		Bus:                "shop",
		PaymentSecret:      "shop/payment-key",
		TemplateParameter:  "/shop/invoices/default-template",
		SigningKey:         "alias/invoice-signing",
		CreditNoteWorkflow: "arn:aws:states:eu-west-1:192325781350:stateMachine:credit-note",
		LogGroup:           "/shop/aws-shop",
		WorkerRole:         "arn:aws:iam::192325781350:role/invoice-worker",
		PaymentQRFunction:  "payment-qr",
	}
}

// Settings say how the apps reach AWS.
type Settings struct {
	// Proxy is Hotline's forward proxy, such as http://hotline:8080. Each
	// app adds its own name as the user name, its integration ID. Empty:
	// no proxy.
	Proxy string
	// SQSEndpoint, S3Endpoint and SNSEndpoint replace AWS's own addresses,
	// for a local SQS, S3 or SNS. Empty: AWS.
	SQSEndpoint, S3Endpoint, SNSEndpoint string
}

// App is one of the shop's apps, with its AWS clients.
type App struct {
	Name  string
	Names Names
	Log   *slog.Logger

	SQS         *sqs.Client
	Files       invoice.Files
	DynamoDB    *dynamodb.Client
	Kinesis     *kinesis.Client
	EventBridge *eventbridge.Client
	SSM         *ssm.Client
	Secrets     *secretsmanager.Client
	KMS         *kms.Client
	Logs        *cloudwatchlogs.Client
	SFN         *sfn.Client
	SNS         *sns.Client
	Lambda      *lambda.Client
	SES         *sesv2.Client

	logStream bool
}

var errProxy = errors.New("the proxy address can't be read")

// ErrNoOrder means the orders table has no row for the order.
var ErrNoOrder = errors.New("no paid order")

// NewApp sets up the named app's clients. Region and credentials come from
// the usual AWS settings; opts can add to them. Only these clients go
// through Hotline, so the SDK's own credential lookups never reach it. The
// invoices app takes on its role through STS, when one is set, and signs
// all its other calls with the role's keys.
func NewApp(ctx context.Context, name string, s Settings, names Names, opts ...func(*config.LoadOptions) error) (App, error) {
	proxy, err := withUser(s.Proxy, name)
	if err != nil {
		return App{}, err
	}
	settings := invoice.Settings{ProxyURL: proxy, Endpoint: s.SQSEndpoint}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return App{}, err
	}
	client, err := settings.HTTPClient()
	if err != nil {
		return App{}, err
	}
	if client != nil {
		cfg.HTTPClient = client
	}
	// Newer SDKs may send DynamoDB and Kinesis calls to a host named after
	// the account, once the keys say which account they belong to, as keys
	// from STS do. This keeps the one host per service the proxy config names.
	cfg.AccountIDEndpointMode = aws.AccountIDEndpointModeDisabled
	if name == "invoices" && names.WorkerRole != "" {
		// One set of keys for every client, fetched on the first call and
		// kept until it runs out, an hour later.
		role := aws.NewCredentialsCache(stscreds.NewAssumeRoleProvider(sts.NewFromConfig(cfg), names.WorkerRole, func(o *stscreds.AssumeRoleOptions) {
			o.RoleSessionName = name
		}))
		cfg.Credentials = role
		opts = append(append([]func(*config.LoadOptions) error{}, opts...), config.WithCredentialsProvider(role))
	}
	queue, err := invoice.NewClient(ctx, settings, opts...)
	if err != nil {
		return App{}, err
	}
	files, err := invoice.NewFilesClient(ctx, invoice.Settings{ProxyURL: proxy, Endpoint: s.S3Endpoint}, opts...)
	if err != nil {
		return App{}, err
	}
	return App{
		Name:        name,
		Names:       names,
		Log:         slog.New(slog.DiscardHandler),
		SQS:         queue,
		Files:       invoice.Files{Client: files, Bucket: names.Bucket},
		DynamoDB:    dynamodb.NewFromConfig(cfg),
		Kinesis:     kinesis.NewFromConfig(cfg),
		EventBridge: eventbridge.NewFromConfig(cfg),
		SSM:         ssm.NewFromConfig(cfg),
		Secrets:     secretsmanager.NewFromConfig(cfg),
		KMS:         kms.NewFromConfig(cfg),
		Logs:        cloudwatchlogs.NewFromConfig(cfg),
		SFN:         sfn.NewFromConfig(cfg),
		SNS: sns.NewFromConfig(cfg, func(o *sns.Options) {
			if s.SNSEndpoint != "" {
				o.BaseEndpoint = aws.String(s.SNSEndpoint)
			}
		}),
		Lambda: lambda.NewFromConfig(cfg),
		SES:    sesv2.NewFromConfig(cfg),
	}, nil
}

// withUser puts the app's name in the proxy address as the user name, the
// integration ID Hotline reads.
func withUser(proxy, name string) (string, error) {
	if proxy == "" {
		return "", nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: %q", errProxy, proxy)
	}
	u.User = url.UserPassword(name, "x")
	return u.String(), nil
}
