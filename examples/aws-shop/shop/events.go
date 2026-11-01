package shop

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	logtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/kinesis"
)

// event is one shop event on the Kinesis stream, as JSON in the record.
type event struct {
	Type     string `json:"type"`
	OrderID  string `json:"orderId"`
	At       string `json:"at"`
	Total    string `json:"total,omitempty"`
	Pages    int    `json:"pages,omitempty"`
	RenderMs *int64 `json:"renderMs,omitempty"`
}

// announce writes a shop event to the stream, keyed by its order so one
// order's events stay in order.
func (a App) announce(ctx context.Context, e event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = a.Kinesis.PutRecord(ctx, &kinesis.PutRecordInput{
		StreamName:   aws.String(a.Names.Stream),
		PartitionKey: aws.String(e.OrderID),
		Data:         data,
	})
	return err
}

// note writes one line to the app's log, and to the shop's log group in
// CloudWatch Logs when there is one, in a log stream named after the app.
func (a *App) note(ctx context.Context, message string, args ...any) error {
	a.Log.Info(message, args...)
	if a.Names.LogGroup == "" {
		return nil
	}
	if !a.logStream {
		_, err := a.Logs.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
			LogGroupName: aws.String(a.Names.LogGroup), LogStreamName: aws.String(a.Name),
		})
		var exists *logtypes.ResourceAlreadyExistsException
		if err != nil && !errors.As(err, &exists) {
			return err
		}
		a.logStream = true
	}
	_, err := a.Logs.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  aws.String(a.Names.LogGroup),
		LogStreamName: aws.String(a.Name),
		LogEvents:     []logtypes.InputLogEvent{{Message: aws.String(message), Timestamp: aws.Int64(time.Now().UnixMilli())}},
	})
	return err
}

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }
