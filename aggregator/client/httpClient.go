package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type HttpClient struct {
	Endpoint string
	client   *http.Client
}

var (
	httpClientDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "http_client_request_duration_seconds",
		Buckets: []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	}, []string{"method", "operation"})
	httpClientRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tollify", Name: "http_requests_total",
	}, []string{"role", "method", "operation", "code"})
)

func NewHttpClient(endpoint string) *HttpClient {
	return NewHttpClientWithTimeout(endpoint, 2*time.Second)
}

func NewHttpClientWithTimeout(endpoint string, timeout time.Duration) *HttpClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 100
	transport.MaxConnsPerHost = 0
	transport.IdleConnTimeout = 90 * time.Second
	transport.ForceAttemptHTTP2 = true
	return &HttpClient{
		Endpoint: strings.TrimRight(endpoint, "/"),
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (c *HttpClient) Aggregate(ctx context.Context, req *types.AggregateRequest) error {
	start := time.Now()
	code := "client_error"
	defer func() {
		httpClientDuration.WithLabelValues(http.MethodPost, "aggregate").Observe(time.Since(start).Seconds())
		httpClientRequests.WithLabelValues("client", http.MethodPost, "aggregate", code).Inc()
	}()
	distance := types.Distance{
		EventID:            req.EventID,
		ProducedAtUnixNano: req.ProducedAtUnixNano,
		OBUID:              int(req.ObuID),
		Value:              req.Value,
	}
	b, err := json.Marshal(distance)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"/agg", bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(r)
	if err != nil {
		return err
	}
	defer closeResponse(resp.Body)
	code = fmt.Sprintf("%d", resp.StatusCode)
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("aggregator returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *HttpClient) GetInvoice(ctx context.Context, id int) (*types.Invoice, error) {
	start := time.Now()
	code := "client_error"
	defer func() {
		httpClientDuration.WithLabelValues(http.MethodGet, "invoice").Observe(time.Since(start).Seconds())
		httpClientRequests.WithLabelValues("client", http.MethodGet, "invoice", code).Inc()
	}()
	endpoint := fmt.Sprintf("%s/%s?obu=%d", c.Endpoint, "invoice", id)

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(r)
	if err != nil {
		return nil, err
	}
	defer closeResponse(resp.Body)
	code = fmt.Sprintf("%d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aggregator returned HTTP %d", resp.StatusCode)
	}
	var inv types.Invoice
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		return nil, err
	}

	return &inv, nil
}

func (c *HttpClient) Close() error {
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}

func closeResponse(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64<<10))
	_ = body.Close()
}
