package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	c "github.com/24aysh/toll-calc/aggregator/client"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	downstreamTimeout, err := time.ParseDuration(envOr("AGGREGATOR_REQUEST_TIMEOUT", "2s"))
	if err != nil || downstreamTimeout <= 0 {
		log.Fatalf("invalid AGGREGATOR_REQUEST_TIMEOUT: %v", err)
	}
	protocol := strings.ToLower(envOr("AGGREGATOR_PROTOCOL", "grpc"))
	var ac c.Client
	switch protocol {
	case "http":
		ac = c.NewHttpClientWithTimeout(envOr("AGGREGATOR_HTTP_ADDR", "http://localhost:4000"), downstreamTimeout)
	case "grpc":
		ac, err = c.NewGrpcClientWithTimeout(envOr("AGGREGATOR_GRPC_ADDR", "localhost:3001"), downstreamTimeout)
	default:
		log.Fatalf("AGGREGATOR_PROTOCOL must be http or grpc, got %q", protocol)
	}
	if err != nil {
		log.Fatal(err)
	}
	defer ac.Close()

	svc := NewLogMiddleware(NewCalcService())
	consumer, err := NewKafkaConsumerWithConfig(KafkaConsumerConfig{
		Brokers:           envOr("KAFKA_BROKERS", "localhost:9092"),
		GroupID:           envOr("KAFKA_GROUP_ID", "tollify-distance-calculator"),
		Topic:             envOr("KAFKA_TOPIC", "obudata"),
		DownstreamTimeout: downstreamTimeout,
		PollInterval:      250 * time.Millisecond,
	}, svc, ac)
	if err != nil {
		log.Fatal(err)
	}
	defer consumer.Close()

	metricsServer := &http.Server{
		Addr:              envOr("CALCULATOR_METRICS_ADDR", ":9102"),
		Handler:           promhttp.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := consumer.Start(ctx); err != nil {
		log.Printf("consumer stopped: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = metricsServer.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
