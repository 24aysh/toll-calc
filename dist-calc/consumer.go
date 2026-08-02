package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/24aysh/toll-calc/aggregator/client"
	"github.com/24aysh/toll-calc/types"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

type KafkaConsumerConfig struct {
	Brokers           string
	GroupID           string
	Topic             string
	DownstreamTimeout time.Duration
	PollInterval      time.Duration
}

type KafkaConsumer struct {
	consumer          *kafka.Consumer
	aggClient         client.Client
	calcService       CalculatorServicer
	downstreamTimeout time.Duration
	pollInterval      time.Duration
	closeOnce         sync.Once
}

var (
	consumerEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "calculator_events_consumed_total",
	})
	consumerErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tollify", Name: "calculator_consumer_errors_total",
	}, []string{"stage"})
	deserializationErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "calculator_deserialization_errors_total",
	})
	distanceDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "distance_calculation_duration_seconds",
		Buckets: []float64{0.00001, 0.000025, 0.00005, 0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01},
	})
	downstreamDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "downstream_aggregation_duration_seconds",
		Buckets: []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2},
	})
	downstreamTimeouts = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "downstream_timeouts_total",
	})
	consumerLag = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "tollify", Name: "kafka_consumer_lag",
	}, []string{"topic", "partition"})
)

func NewKafkaConsumer(topic string, svc CalculatorServicer, ac client.Client) (*KafkaConsumer, error) {
	return NewKafkaConsumerWithConfig(KafkaConsumerConfig{
		Brokers:           "localhost:9092",
		GroupID:           "tollify-distance-calculator",
		Topic:             topic,
		DownstreamTimeout: 2 * time.Second,
		PollInterval:      250 * time.Millisecond,
	}, svc, ac)
}

func NewKafkaConsumerWithConfig(cfg KafkaConsumerConfig, svc CalculatorServicer, ac client.Client) (*KafkaConsumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":  cfg.Brokers,
		"group.id":           cfg.GroupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	})
	if err != nil {
		return nil, err
	}
	if err := c.SubscribeTopics([]string{cfg.Topic}, nil); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &KafkaConsumer{
		consumer:          c,
		calcService:       svc,
		aggClient:         ac,
		downstreamTimeout: cfg.DownstreamTimeout,
		pollInterval:      cfg.PollInterval,
	}, nil
}

func (c *KafkaConsumer) Start(ctx context.Context) error {
	logrus.Info("Kafka consumer started")
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		msg, err := c.consumer.ReadMessage(c.pollInterval)
		if err != nil {
			var kafkaErr kafka.Error
			if errors.As(err, &kafkaErr) && kafkaErr.Code() == kafka.ErrTimedOut {
				continue
			}
			consumerErrors.WithLabelValues("read").Inc()
			logrus.Errorf("Kafka consumer error: %s", err)
			continue
		}
		consumerEvents.Inc()
		c.updateLag(msg)
		// Once an event has been read, let its bounded downstream call finish even
		// if shutdown is requested. The loop stops before accepting more work.
		if err := c.processMessage(context.WithoutCancel(ctx), msg); err != nil {
			consumerErrors.WithLabelValues("process").Inc()
			logrus.Errorf("message processing error: %s", err)
			if seekErr := c.consumer.Seek(msg.TopicPartition, int(c.pollInterval.Milliseconds())); seekErr != nil {
				consumerErrors.WithLabelValues("seek").Inc()
				return fmt.Errorf("rewind failed after processing error: %w", seekErr)
			}
		}
	}
}

func (c *KafkaConsumer) processMessage(parent context.Context, msg *kafka.Message) error {
	var data types.OBUData
	if err := json.Unmarshal(msg.Value, &data); err != nil {
		deserializationErrors.Inc()
		logrus.Errorf("discarding invalid JSON: %s", err)
		_, commitErr := c.consumer.CommitMessage(msg)
		return commitErr
	}
	if data.EventID == "" || data.ProducedAtUnixNano <= 0 || data.OBUID <= 0 {
		deserializationErrors.Inc()
		logrus.Errorf("discarding invalid event: event_id=%q obu_id=%d", data.EventID, data.OBUID)
		_, commitErr := c.consumer.CommitMessage(msg)
		return commitErr
	}
	if err := c.processEvent(parent, data); err != nil {
		return err
	}
	if _, err := c.consumer.CommitMessage(msg); err != nil {
		return fmt.Errorf("commit event %s: %w", data.EventID, err)
	}
	return nil
}

func (c *KafkaConsumer) processEvent(parent context.Context, data types.OBUData) error {
	start := time.Now()
	dist, err := c.calcService.CalculateDist(data)
	distanceDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		return fmt.Errorf("calculate distance: %w", err)
	}
	req := &types.AggregateRequest{
		Value:              dist,
		ObuID:              int32(data.OBUID),
		EventID:            data.EventID,
		ProducedAtUnixNano: data.ProducedAtUnixNano,
	}
	ctx, cancel := context.WithTimeout(parent, c.downstreamTimeout)
	start = time.Now()
	err = c.aggClient.Aggregate(ctx, req)
	downstreamDuration.Observe(time.Since(start).Seconds())
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		downstreamTimeouts.Inc()
	}
	cancel()
	if err != nil {
		return fmt.Errorf("aggregate event %s: %w", data.EventID, err)
	}
	return nil
}

func (c *KafkaConsumer) updateLag(msg *kafka.Message) {
	if msg.TopicPartition.Topic == nil {
		return
	}
	_, high, err := c.consumer.GetWatermarkOffsets(*msg.TopicPartition.Topic, msg.TopicPartition.Partition)
	if err != nil {
		return
	}
	lag := high - int64(msg.TopicPartition.Offset) - 1
	if lag < 0 {
		lag = 0
	}
	consumerLag.WithLabelValues(*msg.TopicPartition.Topic, strconv.Itoa(int(msg.TopicPartition.Partition))).Set(float64(lag))
}

func (c *KafkaConsumer) Close() error {
	var err error
	c.closeOnce.Do(func() { err = c.consumer.Close() })
	return err
}
