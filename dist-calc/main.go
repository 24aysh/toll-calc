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
	ac := c.NewClient(aggregatorEndpoint)
	kafkaConsumer, err := NewKafkaConsumer(kafkaTopic, svc, ac)
	if err != nil {
		log.Fatal(err)
	}
	kafkaConsumer.Start()
}
