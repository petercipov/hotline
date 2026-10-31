package invoice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// ErrNoInvoice means the bucket holds no invoice for the order.
var ErrNoInvoice = errors.New("no invoice stored for this order")

// NewFilesClient makes an S3 client, set up like the SQS client in
// NewClient: only this client goes through Hotline. With an endpoint, such
// as a local S3, the bucket goes in the path, as local S3s expect; AWS gets
// it in the host name.
func NewFilesClient(ctx context.Context, s Settings, opts ...func(*config.LoadOptions) error) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}
	client, err := s.httpClient()
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		if s.Endpoint != "" {
			o.BaseEndpoint = aws.String(s.Endpoint)
			o.UsePathStyle = true
		}
		if client != nil {
			o.HTTPClient = client
		}
	}), nil
}

// Files keeps invoice PDFs in an S3 bucket, one per order, named like
// A-1001.pdf.
type Files struct {
	Client *s3.Client
	Bucket string
}

// File is one stored invoice.
type File struct {
	Key      string
	Size     int64
	Modified time.Time
}

// Key is the name an order's invoice is stored under.
func Key(orderID string) string { return orderID + ".pdf" }

// Store puts an order's invoice PDF in the bucket.
func (f Files) Store(ctx context.Context, orderID string, pdf []byte) error {
	_, err := f.Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &f.Bucket,
		Key:         aws.String(Key(orderID)),
		Body:        bytes.NewReader(pdf),
		ContentType: aws.String("application/pdf"),
	})
	if err != nil {
		return fmt.Errorf("store %s: %w", Key(orderID), err)
	}
	return nil
}

// Fetch gets an order's invoice PDF from the bucket.
func (f Files) Fetch(ctx context.Context, orderID string) ([]byte, error) {
	out, err := f.Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &f.Bucket, Key: aws.String(Key(orderID))})
	var missing *types.NoSuchKey
	if errors.As(err, &missing) {
		return nil, fmt.Errorf("%w: %s", ErrNoInvoice, orderID)
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = out.Body.Close() }()
	return io.ReadAll(out.Body)
}

// List lists every stored invoice, in the order of their names.
func (f Files) List(ctx context.Context) ([]File, error) {
	var files []File
	pages := s3.NewListObjectsV2Paginator(f.Client, &s3.ListObjectsV2Input{Bucket: &f.Bucket})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, o := range page.Contents {
			files = append(files, File{Key: aws.ToString(o.Key), Size: aws.ToInt64(o.Size), Modified: aws.ToTime(o.LastModified)})
		}
	}
	return files, nil
}

// most is how many files S3 deletes in one call.
const most = 1000

// Cleanup deletes the invoices stored before a time, up to a thousand in
// each call, and returns their names.
func (f Files) Cleanup(ctx context.Context, before time.Time) ([]string, error) {
	files, err := f.List(ctx)
	if err != nil {
		return nil, err
	}
	var old []string
	for _, file := range files {
		if file.Modified.Before(before) {
			old = append(old, file.Key)
		}
	}
	for start := 0; start < len(old); start += most {
		batch := old[start:min(start+most, len(old))]
		ids := make([]types.ObjectIdentifier, len(batch))
		for i, key := range batch {
			ids[i] = types.ObjectIdentifier{Key: aws.String(key)}
		}
		_, err := f.Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: &f.Bucket,
			Delete: &types.Delete{Objects: ids, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return nil, err
		}
	}
	return old, nil
}
