package main

import (
	"context"
	"fmt"
	"net"

	"github.com/24aysh/toll-calc/types"
	"google.golang.org/grpc"
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
	dist := types.Distance{
		OBUID: int(req.ObuID),
		Value: req.Value,
		Unix:  req.Unix,
	}
	return &types.None{}, s.svc.AggregateDistance(dist)

}

func makeGRPCTransport(listenAddr string, svc Aggregator) error {
	fmt.Println("GRPC Transport Running")
	// make a TCP listener
	l, err := net.Listen("tcp", listenAddr)

	if err != nil {
		return err
	}
	defer l.Close()
	server := grpc.NewServer([]grpc.ServerOption{}...)

	types.RegisterAggregatorServer(server, NewGRPCAggregatorServer(svc))

	return server.Serve(l)

}
