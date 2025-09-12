package setup_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
)

// example from https://bump.sh/bump-examples/doc/train-travel-api/operation/operation-get-stations
type EgressClient struct {
	rand   *rand.Rand
	client *http.Client
}

func NewEgressClient(proxyURl *url.URL, seed int64) *EgressClient {
	return &EgressClient{
		rand: rand.New(rand.NewSource(seed)),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURl),
			},
		},
	}
}

var errInvalidRequest = errors.New("invalid request")

func (s *EgressClient) SendTraffic(integrationID string, targetURL string) (int, error) {
	switch s.rand.Intn(3) {
	case 0:
		return s.SendGet(integrationID, s.rand.Int31(), targetURL)
	case 1:
		return s.SendPost(integrationID, s.rand.Int31(), targetURL)
	case 2:
		return s.SendDelete(integrationID, s.rand.Int31(), targetURL)
	default:
		return 0, errInvalidRequest
	}
}

func (s *EgressClient) SendGet(integrationID string, rand int32, targetURL string) (int, error) {
	req, createErr := http.NewRequestWithContext(context.Background(), "GET", targetURL+"/bookings?page=0&limit=50&country=uk", nil)
	if createErr != nil {
		return 0, createErr
	}

	req.Header.Set("x-request-id", fmt.Sprintf("req-%d", rand))
	req.Header.Set("User-Agent", integrationID)

	resp, respErr := s.client.Do(req)
	if respErr != nil {
		return 0, respErr
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode, nil
}

func (s *EgressClient) SendPost(integrationID string, rand int32, targetURL string) (int, error) {

	body := `{
		"tripId": "4f4e4e1-c824-4d63-b37a-d8d698862f1d",
		"passengerName": "John Doe"
	}'`
	req, createErr := http.NewRequestWithContext(context.Background(), "POST", targetURL+"/bookings", bytes.NewBuffer([]byte(body)))
	if createErr != nil {
		return 0, createErr
	}

	req.Header.Set("x-request-id", fmt.Sprintf("req-%d", rand))
	req.Header.Set("User-Agent", integrationID)

	resp, respErr := s.client.Do(req)
	if respErr != nil {
		return 0, respErr
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode, nil
}

func (s *EgressClient) SendDelete(integrationID string, rand int32, targetURL string) (int, error) {
	req, createErr := http.NewRequestWithContext(
		context.Background(),
		"DELETE",
		fmt.Sprintf("%s/bookings/%s", targetURL, "4f4e4e1-c824-4d63-b37a-d8d698862f1d"),
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
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode, nil
}
