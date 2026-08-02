package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	httpAddr := flag.String("http-addr", envOr("AGGREGATOR_HTTP_LISTEN_ADDR", ":4000"), "HTTP listen address")
	grpcAddr := flag.String("grpc-addr", envOr("AGGREGATOR_GRPC_LISTEN_ADDR", ":3001"), "gRPC listen address")
	flag.Parse()

	store := NewMemoryStore()
	var svc Aggregator = NewInvoiceAggregator(store)
	svc = NewMetricMiddleware(svc)
	svc = NewLogMiddleware(svc)

	listener, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatal(err)
	}
	grpcServer := newGRPCServer(svc)
	httpServer := &http.Server{
		Addr:              *httpAddr,
		Handler:           newHTTPHandler(svc),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
		ConnState: func(_ net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				aggregatorOpenConnections.WithLabelValues("http").Inc()
			case http.StateClosed, http.StateHijacked:
				aggregatorOpenConnections.WithLabelValues("http").Dec()
			}
		},
	}
	errorsCh := make(chan error, 2)
	go func() { errorsCh <- grpcServer.Serve(listener) }()
	go func() { errorsCh <- httpServer.ListenAndServe() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errorsCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("aggregator transport stopped: %v", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown: %v", err)
	}
	grpcStopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcStopped)
	}()
	select {
	case <-grpcStopped:
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
