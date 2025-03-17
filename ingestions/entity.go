package ingestions

import (
	"net/url"
	"time"
)

type HttpRequest struct {
	ID              string
	IntegrationID   string
	ProtocolVersion string
	Method          string
	StatusCode      string
	URL             *url.URL
	StartTime       time.Time
	EndTime         time.Time
	ErrorType       string
}
