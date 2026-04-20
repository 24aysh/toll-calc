package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/24aysh/toll-calc/aggregator/client"
	"github.com/24aysh/toll-calc/types"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/sirupsen/logrus"
)

type KafkaConsumer struct {
	consumer    *kafka.Consumer
	aggClient   client.Client
	isRunning   bool
	calcService CalculatorServicer
}

func NewKafkaConsumer(topic string, svc CalculatorServicer, ac client.Client) (*KafkaConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": "localhost",
		"group.id":          "myGroup",
		"auto.offset.reset": "earliest",
	})

	if err != nil {
		return nil, err
	}

	err = c.SubscribeTopics([]string{topic}, nil)

	if err != nil {
		return nil, err
	}
	return &KafkaConsumer{
		consumer:    c,
		calcService: svc,
		aggClient:   ac,
	}, nil
}

func (c *KafkaConsumer) Start() {
	logrus.Info("Kafka Consumer Started")
	c.isRunning = true
	c.readMessageLoop()
}

func (c *KafkaConsumer) readMessageLoop() {
	for c.isRunning {
		msg, err := c.consumer.ReadMessage(-1)
		if err != nil {
			logrus.Errorf("Kafka produced error : %s", err)
			continue
		}
		var data types.OBUData
		if err = json.Unmarshal(msg.Value, &data); err != nil {
			logrus.Errorf("JSON Unmarshal Error: %s", err)
			continue
		}
		dist, err := c.calcService.CalculateDist(data)
		if err != nil {
			logrus.Errorf("Dist Service error: %s", err)
			continue
		}
		req := &types.AggregateRequest{
			Value: dist,
			ObuID: int32(data.OBUID),
			Unix:  time.Now().UnixNano(),
		}
		if err := c.aggClient.Aggregate(context.Background(), req); err != nil {
			logrus.Errorf("Client Error :%s", err)
			continue
		}
	}

}
