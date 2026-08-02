package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/24aysh/toll-calc/types"
	"google.golang.org/grpc"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestHTTPClientUsesContextAndCorrectMethods(t *testing.T) {
	client := NewHttpClient("http://aggregator")
	defer client.Close()
	client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"obu_id":3,"total_distance":7}`))}
		switch request.URL.Path {
		case "/agg":
			if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
				t.Errorf("aggregate method=%s content-type=%q", request.Method, request.Header.Get("Content-Type"))
			}
			response.StatusCode = http.StatusAccepted
		case "/invoice":
			if request.Method != http.MethodGet {
				t.Errorf("invoice method=%s", request.Method)
			}
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		return response, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Aggregate(ctx, &types.AggregateRequest{EventID: "id", ObuID: 3}); err != nil {
		t.Fatal(err)
	}
	invoice, err := client.GetInvoice(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	if invoice.OBUID != 3 || invoice.TotalDist != 7 {
		t.Fatalf("invoice=%+v", invoice)
	}
}

func TestHTTPClientHonorsContext(t *testing.T) {
	client := NewHttpClient("http://aggregator")
	defer client.Close()
	client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := client.Aggregate(ctx, &types.AggregateRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want deadline exceeded", err)
	}
}

type blockingGRPCClient struct{}

func (blockingGRPCClient) Aggregate(ctx context.Context, _ *types.AggregateRequest, _ ...grpc.CallOption) (*types.None, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestGRPCClientAppliesPerCallDeadline(t *testing.T) {
	client := &GrpcClient{client: blockingGRPCClient{}, timeout: 20 * time.Millisecond}
	err := client.Aggregate(context.Background(), &types.AggregateRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want deadline exceeded", err)
	}
}
