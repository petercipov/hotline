package ingestions

import (
	"hotline/integrations"
	"hotline/servicelevels"
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
	CorrelationID   string
}

func ToSLORequest(requests []*HttpRequest, now time.Time) []*servicelevels.HttpReqsMessage {
	byIntegrationId := make(map[integrations.ID][]*HttpRequest)
	for _, request := range requests {
		byIntegrationId[request.IntegrationID] = append(byIntegrationId[request.IntegrationID], request)
	}
	var result []*servicelevels.HttpReqsMessage
	for integrationID, httpRequests := range byIntegrationId {
		var reqs []*servicelevels.HttpRequest
		for _, httpRequest := range httpRequests {
			latency := servicelevels.LatencyMs(
				httpRequest.EndTime.Sub(httpRequest.StartTime).Milliseconds())
			state := httpRequest.ErrorType
			if len(httpRequest.StatusCode) > 0 {
				state = httpRequest.StatusCode
			}
			reqs = append(reqs, &servicelevels.HttpRequest{
				Latency: latency,
				State:   state,
				Method:  httpRequest.Method,
				URL:     httpRequest.URL,
			})
		}
		result = append(result, &servicelevels.HttpReqsMessage{
			Now:  now,
			ID:   integrationID,
			Reqs: reqs,
		})
	}
	return result
}
