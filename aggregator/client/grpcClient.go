package client

import (
	"context"

	"github.com/24aysh/toll-calc/types"
	"google.golang.org/grpc"
)

type GrpcClient struct {
	Endpoint string
	client   types.AggregatorClient
}

func NewGrpcClient(ep string) (*GrpcClient, error) {

	conn, err := grpc.NewClient(ep, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	c := types.NewAggregatorClient(conn)

	return &GrpcClient{
		Endpoint: ep,
		client:   c,
	}, nil
}

func (g *GrpcClient) Aggregate(ctx context.Context, req *types.AggregateRequest) error {
	_, err := g.client.Aggregate(ctx, req)
	return err
}
