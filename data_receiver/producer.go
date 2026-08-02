package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type DataProducer interface {
	ProduceData(context.Context, types.OBUData) error
	Close() error
}

type KafkaProducer struct {
	producer *kafka.Producer
	topic    string
}

var kafkaAcknowledgementDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Namespace: "tollify", Name: "receiver_kafka_acknowledgement_duration_seconds",
	Buckets: []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5},
})

func (p *KafkaProducer) ProduceData(ctx context.Context, data types.OBUData) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	delivery := make(chan kafka.Event, 1)
	start := time.Now()
	err = p.producer.Produce(newKafkaMessage(p.topic, data, b), delivery)
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case event := <-delivery:
		kafkaAcknowledgementDuration.Observe(time.Since(start).Seconds())
		message, ok := event.(*kafka.Message)
		if !ok {
			return fmt.Errorf("unexpected Kafka delivery event %T", event)
		}
		return message.TopicPartition.Error
	}
}

func newKafkaMessage(topic string, data types.OBUData, value []byte) *kafka.Message {
	return &kafka.Message{
		TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: kafka.PartitionAny},
		Key:            []byte(strconv.Itoa(data.OBUID)),
		Value:          value,
	}
}

func NewKafkaProducer() (DataProducer, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers": envOr("KAFKA_BROKERS", "localhost:9092"),
		"acks":              "all",
	})
	if err != nil {
		return nil, err
	}
	return &KafkaProducer{producer: p, topic: envOr("KAFKA_TOPIC", "obudata")}, nil
}

func (p *KafkaProducer) Close() error {
	if remaining := p.producer.Flush(5000); remaining > 0 {
		p.producer.Close()
		return fmt.Errorf("%d Kafka messages were not delivered before shutdown", remaining)
	}
	p.producer.Close()
	return nil
}
