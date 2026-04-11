package main

import "log"

type DistanceCalc struct {
}

const kafkaTopic = "obudata"

func main() {
	svc := NewCalcService()
	kafkaConsumer, err := NewKafkaConsumer(kafkaTopic, svc)
	if err != nil {
		log.Fatal(err)
	}
	kafkaConsumer.Start()
}
