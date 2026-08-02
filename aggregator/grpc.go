package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstats "google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

var (
	grpcServerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "grpc_server_request_duration_seconds",
		Buckets: []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"method"})
	grpcServerRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tollify", Name: "grpc_requests_total",
	}, []string{"role", "method", "code"})
)

type GRPCAggregatorServer struct {
	types.UnimplementedAggregatorServer
	svc Aggregator
}

func NewGRPCAggregatorServer(svc Aggregator) *GRPCAggregatorServer {
	return &GRPCAggregatorServer{
		svc: svc,
	}
}

func (s *GRPCAggregatorServer) Aggregate(ctx context.Context, req *types.AggregateRequest) (*types.None, error) {
	aggregatorProtocolEvents.WithLabelValues("grpc").Inc()
	dist := types.Distance{
		OBUID:              int(req.ObuID),
		Value:              req.Value,
		EventID:            req.EventID,
		ProducedAtUnixNano: req.ProducedAtUnixNano,
	}
	_, err := s.svc.AggregateDistance(dist)
	if errors.Is(err, ErrInvalidDistance) {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &types.None{}, err

}

func makeGRPCTransport(listenAddr string, svc Aggregator) error {
	fmt.Println("GRPC Transport Running")
	// make a TCP listener
	l, err := net.Listen("tcp", listenAddr)

	if err != nil {
		return err
	}
	defer l.Close()
	server := newGRPCServer(svc)

	return server.Serve(l)

}

func newGRPCServer(svc Aggregator) *grpc.Server {
	server := grpc.NewServer(
		grpc.UnaryInterceptor(grpcMetricsInterceptor),
		grpc.StatsHandler(grpcConnectionStats{}),
	)
	types.RegisterAggregatorServer(server, NewGRPCAggregatorServer(svc))
	return server
}

type grpcConnectionStats struct{}

func (grpcConnectionStats) TagRPC(ctx context.Context, _ *grpcstats.RPCTagInfo) context.Context {
	return ctx
}

func (grpcConnectionStats) HandleRPC(context.Context, grpcstats.RPCStats) {}

func (grpcConnectionStats) TagConn(ctx context.Context, _ *grpcstats.ConnTagInfo) context.Context {
	return ctx
}

func (grpcConnectionStats) HandleConn(_ context.Context, stat grpcstats.ConnStats) {
	switch stat.(type) {
	case *grpcstats.ConnBegin:
		aggregatorOpenConnections.WithLabelValues("grpc").Inc()
	case *grpcstats.ConnEnd:
		aggregatorOpenConnections.WithLabelValues("grpc").Dec()
	}
}

func grpcMetricsInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	code := status.Code(err).String()
	grpcServerDuration.WithLabelValues(info.FullMethod).Observe(time.Since(start).Seconds())
	grpcServerRequests.WithLabelValues("server", info.FullMethod, code).Inc()
	return resp, err
}
