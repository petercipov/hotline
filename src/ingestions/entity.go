package ingestions

import (
	"hotline/src/integrations"
	"net/url"
	"time"
)

type HttpRequest struct {
	ID              string
	IntegrationID   integrations.ID
	ProtocolVersion string
	Method          string
	StatusCode      string
	URL             *url.URL
	StartTime       time.Time
	EndTime         time.Time
	ErrorType       string
}
