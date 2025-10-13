package ingestions_test

import (
	"hotline/clock"
	"hotline/http"
	"hotline/ingestions"
	"hotline/servicelevels"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Entities", func() {
	It("should transform empty array", func() {
		slos := ingestions.ToSLORequestMessage(nil, clock.ParseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(BeEmpty())
	})

	It("should ingested single request", func() {
		message := ingestions.ToSLOSingleRequestMessage(&ingestions.HttpRequest{
			ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
			IntegrationID:   "integration.com",
			ProtocolVersion: "1.1",
			Method:          "POST",
			StatusCode:      "200",
			URL:             newUrl("https://integration.com/order/123?param1=value1"),
			StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
			EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
		}, clock.ParseTime("2018-12-13T14:51:00Z"))

		Expect(message).To(Equal(&servicelevels.IngestRequestsMessage{
			ID:  "integration.com",
			Now: clock.ParseTime("2018-12-13T14:51:00Z"),
			Reqs: []*servicelevels.HttpRequest{
				{
					Latency: servicelevels.LatencyMs(1000),
					State:   "200",
					Locator: http.RequestLocator{
						Method: "POST",
						Path:   "/order/123",
						Host:   "integration.com",
						Port:   443,
					},
				},
			},
		}))
	})

	It("should ingested request", func() {
		slos := ingestions.ToSLORequestMessage([]*ingestions.HttpRequest{
			{
				ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID:   "integration.com",
				ProtocolVersion: "1.1",
				Method:          "POST",
				StatusCode:      "200",
				URL:             newUrl("https://integration.com/order/123?param1=value1"),
				StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
				EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
			},
		}, clock.ParseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(HaveLen(1))
		Expect(slos[0]).To(Equal(&servicelevels.IngestRequestsMessage{
			ID:  "integration.com",
			Now: clock.ParseTime("2018-12-13T14:51:00Z"),
			Reqs: []*servicelevels.HttpRequest{
				{
					Latency: servicelevels.LatencyMs(1000),
					State:   "200",
					Locator: http.RequestLocator{
						Method: "POST",
						Path:   "/order/123",
						Host:   "integration.com",
						Port:   443,
					},
				},
			},
		}))
	})

	It("should ingested request with port", func() {
		slos := ingestions.ToSLORequestMessage([]*ingestions.HttpRequest{
			{
				ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID:   "integration.com",
				ProtocolVersion: "1.1",
				Method:          "POST",
				StatusCode:      "200",
				URL:             newUrl("https://integration.com:5432/order/123?param1=value1"),
				StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
				EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
			},
		}, clock.ParseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(HaveLen(1))
		Expect(slos[0]).To(Equal(&servicelevels.IngestRequestsMessage{
			ID:  "integration.com",
			Now: clock.ParseTime("2018-12-13T14:51:00Z"),
			Reqs: []*servicelevels.HttpRequest{
				{
					Latency: servicelevels.LatencyMs(1000),
					State:   "200",
					Locator: http.RequestLocator{
						Method: "POST",
						Path:   "/order/123",
						Host:   "integration.com",
						Port:   5432,
					},
				},
			},
		}))
	})

	It("should ingested error request", func() {
		slos := ingestions.ToSLORequestMessage([]*ingestions.HttpRequest{
			{
				ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID:   "integration.com",
				ProtocolVersion: "1.1",
				Method:          "POST",
				ErrorType:       "timeout",
				URL:             newUrl("https://integration.com/order/123?param1=value1"),
				StartTime:       clock.ParseTime("2018-12-13T14:51:00Z"),
				EndTime:         clock.ParseTime("2018-12-13T14:51:01Z"),
			},
		}, clock.ParseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(HaveLen(1))
		Expect(slos[0]).To(Equal(&servicelevels.IngestRequestsMessage{
			ID:  "integration.com",
			Now: clock.ParseTime("2018-12-13T14:51:00Z"),
			Reqs: []*servicelevels.HttpRequest{
				{
					Latency: servicelevels.LatencyMs(1000),
					State:   "timeout",
					Locator: http.RequestLocator{
						Method: "POST",
						Path:   "/order/123",
						Host:   "integration.com",
						Port:   443,
					},
				},
			},
		}))
	})
})

func newUrl(s string) *url.URL {
	parsedUrl, parseErr := url.Parse(s)
	Expect(parseErr).ToNot(HaveOccurred())
	return parsedUrl
}
