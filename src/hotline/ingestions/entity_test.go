package ingestions

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
	"net/url"
	"time"
)

var _ = Describe("Entities", func() {
	It("should transform empty array", func() {
		slos := ToSLORequest(nil, parseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(HaveLen(0))
	})

	It("should ingested request", func() {
		slos := ToSLORequest([]*HttpRequest{
			{
				ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID:   "integration.com",
				ProtocolVersion: "1.1",
				Method:          "POST",
				StatusCode:      "200",
				URL:             newUrl("https://integration.com/order/123?param1=value1"),
				StartTime:       parseTime("2018-12-13T14:51:00Z"),
				EndTime:         parseTime("2018-12-13T14:51:01Z"),
			},
		}, parseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(HaveLen(1))
		Expect(slos[0]).To(Equal(&servicelevels.HttpReqsMessage{
			ID:  "integration.com",
			Now: parseTime("2018-12-13T14:51:00Z"),
			Reqs: []*servicelevels.HttpRequest{
				{
					Latency: servicelevels.LatencyMs(1000),
					State:   "200",
					Method:  "POST",
					URL:     newUrl("https://integration.com/order/123?param1=value1"),
				},
			},
		}))
	})

	It("should ingested error request", func() {
		slos := ToSLORequest([]*HttpRequest{
			{
				ID:              "5B8EFFF798038103D269B633813FC60C0:EEE19B7EC3C1B1740",
				IntegrationID:   "integration.com",
				ProtocolVersion: "1.1",
				Method:          "POST",
				ErrorType:       "timeout",
				URL:             newUrl("https://integration.com/order/123?param1=value1"),
				StartTime:       parseTime("2018-12-13T14:51:00Z"),
				EndTime:         parseTime("2018-12-13T14:51:01Z"),
			},
		}, parseTime("2018-12-13T14:51:00Z"))
		Expect(slos).To(HaveLen(1))
		Expect(slos[0]).To(Equal(&servicelevels.HttpReqsMessage{
			ID:  "integration.com",
			Now: parseTime("2018-12-13T14:51:00Z"),
			Reqs: []*servicelevels.HttpRequest{
				{
					Latency: servicelevels.LatencyMs(1000),
					State:   "timeout",
					Method:  "POST",
					URL:     newUrl("https://integration.com/order/123?param1=value1"),
				},
			},
		}))
	})
})

func parseTime(nowString string) time.Time {
	now, parseErr := time.Parse(time.RFC3339, nowString)
	Expect(parseErr).NotTo(HaveOccurred())
	return now
}

func newUrl(s string) *url.URL {
	parsedUrl, parseErr := url.Parse(s)
	Expect(parseErr).ToNot(HaveOccurred())
	return parsedUrl
}
