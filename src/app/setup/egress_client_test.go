package setup_test

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
)

// example from https://bump.sh/bump-examples/doc/train-travel-api/operation/operation-get-stations
type EgressClient struct {
	URL    string
	rand   *rand.Rand
	client *http.Client
}

func NewEgressClient(proxyURl *url.URL, targetURL string, seed int64) *EgressClient {

	return &EgressClient{
		URL:  targetURL,
		rand: rand.New(rand.NewSource(seed)),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURl),
			},
		},
	}
}

func (s *EgressClient) SendTraffic(integrationID string) (int, error) {
	switch s.rand.Intn(3) {
	case 0:
		return s.SendGet(integrationID, s.rand.Int31())
	case 1:
		return s.SendPost(integrationID, s.rand.Int31())
	case 2:
		return s.SendDelete(integrationID, s.rand.Int31())
	default:
		return 0, fmt.Errorf("invalid request")
	}
}

func (s *EgressClient) SendGet(integrationID string, rand int32) (int, error) {
	req, createErr := http.NewRequest("GET", fmt.Sprintf("%s/bookings?page=0&limit=50&country=uk", s.URL), nil)
	if createErr != nil {
		return 0, createErr
	}

	req.Header.Set("x-request-id", fmt.Sprintf("req-%d", rand))
	req.Header.Set("User-Agent", integrationID)

	resp, respErr := s.client.Do(req)
	if respErr != nil {
		return 0, respErr
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (s *EgressClient) SendPost(integrationID string, rand int32) (int, error) {

	body := `{
		"tripId": "4f4e4e1-c824-4d63-b37a-d8d698862f1d",
		"passengerName": "John Doe"
	}'`
	req, createErr := http.NewRequest("POST", fmt.Sprintf("%s/bookings", s.URL), bytes.NewBuffer([]byte(body)))
	if createErr != nil {
		return 0, createErr
	}

	req.Header.Set("x-request-id", fmt.Sprintf("req-%d", rand))
	req.Header.Set("User-Agent", integrationID)

	resp, respErr := s.client.Do(req)
	if respErr != nil {
		return 0, respErr
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func (s *EgressClient) SendDelete(integrationID string, rand int32) (int, error) {
	req, createErr := http.NewRequest(
		"DELETE",
		fmt.Sprintf("%s/bookings/%s", s.URL, "4f4e4e1-c824-4d63-b37a-d8d698862f1d"),
		nil)
	if createErr != nil {
		return 0, createErr
	}

	req.Header.Set("x-request-id", fmt.Sprintf("req-%d", rand))
	req.Header.Set("User-Agent", integrationID)

	resp, respErr := s.client.Do(req)
	if respErr != nil {
		return 0, respErr
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
