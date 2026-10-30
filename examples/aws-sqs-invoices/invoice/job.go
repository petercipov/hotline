// Package invoice is the example's own logic: the invoice job message, its
// check, the PDF it asks for, and sending and receiving it over SQS.
package invoice

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Schema is the JSON Schema of the job message. Hotline's overlay for the
// invoices queue carries the same schema.
//
//go:embed invoice-job.schema.json
var Schema []byte

// ErrNotJSON and ErrSchema say why a message was refused.
var (
	ErrNotJSON = errors.New("the message is not JSON")
	ErrSchema  = errors.New("the message does not match the invoice job schema")
)

func compile() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(Schema))
	if err != nil {
		panic(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("invoice-job.schema.json", doc); err != nil {
		panic(err)
	}
	return c.MustCompile("invoice-job.schema.json")
}

// Job asks the worker to render the invoice PDF of one order.
type Job struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventID       string    `json:"eventId"`
	OrderID       string    `json:"orderId"`
	TemplateID    string    `json:"templateId"`
	Language      string    `json:"language"`
	RequestedAt   time.Time `json:"requestedAt"`
	SendTo        string    `json:"sendTo,omitempty"`
}

// NewJob makes a version 1 job with a fresh event ID.
func NewJob(orderID, templateID, language, sendTo string, now time.Time) Job {
	return Job{
		SchemaVersion: 1,
		EventID:       newUUID(),
		OrderID:       orderID,
		TemplateID:    templateID,
		Language:      language,
		RequestedAt:   now,
		SendTo:        sendTo,
	}
}

// Check reads a message body and checks it against the schema.
func Check(body []byte) (Job, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return Job{}, ErrNotJSON
	}
	if err := compile().Validate(doc); err != nil {
		return Job{}, fmt.Errorf("%w: %s", ErrSchema, leaves(err))
	}
	var job Job
	err = json.Unmarshal(body, &job)
	return job, err
}

// leaves names each place that failed, such as "$.orderId required", in the
// words Hotline uses, and never the value that was sent.
func leaves(err error) string {
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return err.Error()
	}
	var out []string
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		place := strings.Join(append([]string{"$"}, e.InstanceLocation...), ".")
		switch k := e.ErrorKind.(type) {
		case *kind.Required:
			for _, name := range k.Missing {
				out = append(out, place+"."+name+" required")
			}
		case *kind.AdditionalProperties:
			for _, name := range k.Properties {
				out = append(out, place+"."+name+" additionalProperties")
			}
		default:
			path := k.KeywordPath()
			out = append(out, place+" "+path[len(path)-1])
		}
	}
	walk(invalid)
	return strings.Join(out, "; ")
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
