package main

import (
	"context"
	"log"
	"time"

	"github.com/24aysh/toll-calc/aggregator/client"
	"github.com/24aysh/toll-calc/types"
)

func main() {
	c, err := client.NewGrpcClient(":3001")
	if err != nil {
		log.Fatal(err)
	}
	if err := c.Aggregate(context.Background(), &types.AggregateRequest{
		ObuID: 1,
		Value: 12.3,
		Unix:  time.Now().UnixNano(),
	}); err != nil {
		log.Fatal(err)
	}

}
