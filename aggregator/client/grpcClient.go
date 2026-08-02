package client

import (
	"context"
	"fmt"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type GrpcClient struct {
	Endpoint string
	client   types.AggregatorClient
	conn     *grpc.ClientConn
	timeout  time.Duration
}

var (
	grpcClientDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "grpc_client_request_duration_seconds",
		Buckets: []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	}, []string{"method"})
	grpcClientRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tollify", Name: "grpc_requests_total",
	}, []string{"role", "method", "code"})
)

func NewGrpcClient(ep string) (*GrpcClient, error) {
	return NewGrpcClientWithTimeout(ep, 2*time.Second)
}

func NewGrpcClientWithTimeout(ep string, timeout time.Duration) (*GrpcClient, error) {
	conn, err := grpc.NewClient(ep, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	c := types.NewAggregatorClient(conn)

	return &GrpcClient{
		Endpoint: ep,
		client:   c,
		conn:     conn,
		timeout:  timeout,
	}, nil
}

func (g *GrpcClient) Aggregate(ctx context.Context, req *types.AggregateRequest) error {
	callCtx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	start := time.Now()
	_, err := g.client.Aggregate(callCtx, req)
	code := status.Code(err)
	grpcClientDuration.WithLabelValues("Aggregate").Observe(time.Since(start).Seconds())
	grpcClientRequests.WithLabelValues("client", "Aggregate", code.String()).Inc()
	if err != nil {
		return fmt.Errorf("gRPC aggregate (%s): %w", code, err)
	}
	return nil
}

func (g *GrpcClient) GetInvoice(context.Context, int) (*types.Invoice, error) {
	return nil, status.Error(codes.Unimplemented, "invoice lookup is only exposed over HTTP")
}

func (g *GrpcClient) Close() error { return g.conn.Close() }
