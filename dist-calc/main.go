package main

import (
	"log"

	c "github.com/24aysh/toll-calc/aggregator/client"
)

type DistanceCalc struct {
}

const (
	kafkaTopic         = "obudata"
	aggregatorEndpoint = "http://localhost:3000/agg"
)

func main() {
	svc := NewCalcService()
	svc = NewLogMiddleware(svc)
	ac := c.NewHttpClient(aggregatorEndpoint)

	// gc, err := c.NewGrpcClient(aggregatorEndpoint)
	// if err != nil {
	// 	log.Fatal(err)
	// }

	kafkaConsumer, err := NewKafkaConsumer(kafkaTopic, svc, ac)
	if err != nil {
		log.Fatal(err)
	}
	kafkaConsumer.Start()
}
