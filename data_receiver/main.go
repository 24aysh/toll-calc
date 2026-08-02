package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type DataReceiver struct {
	prod           DataProducer
	upgrader       websocket.Upgrader
	readLimit      int64
	readTimeout    time.Duration
	produceTimeout time.Duration
	connectionsMu  sync.Mutex
	connections    map[*websocket.Conn]struct{}
}

var (
	activeWebSockets = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "tollify", Name: "receiver_active_websocket_connections",
	})
	receiverEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "receiver_events_received_total",
	})
	receiverInvalidEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "receiver_invalid_events_total",
	})
	kafkaEnqueueFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "receiver_kafka_enqueue_failures_total",
	})
)

func main() {
	recv, err := NewDataReceiver()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := recv.prod.Close(); err != nil {
			log.Printf("producer shutdown: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", recv.handleWS)
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:              envOr("RECEIVER_HTTP_ADDR", ":30000"),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	server.RegisterOnShutdown(recv.closeConnections)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("receiver server: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("receiver shutdown: %v", err)
	}
}

func (dr *DataReceiver) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := dr.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	activeWebSockets.Inc()
	dr.connectionsMu.Lock()
	dr.connections[conn] = struct{}{}
	dr.connectionsMu.Unlock()
	defer activeWebSockets.Dec()
	defer func() {
		dr.connectionsMu.Lock()
		delete(dr.connections, conn)
		dr.connectionsMu.Unlock()
		_ = conn.Close()
	}()
	dr.wsReceiveLoop(r.Context(), conn)
}

func NewDataReceiver() (*DataReceiver, error) {
	producer, err := NewKafkaProducer()
	if err != nil {
		return nil, err
	}
	return NewDataReceiverWithProducer(NewLogMiddleware(producer)), nil
}

func NewDataReceiverWithProducer(producer DataProducer) *DataReceiver {
	readLimit := int64(1 << 20)
	if value := os.Getenv("WS_READ_LIMIT_BYTES"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			readLimit = parsed
		}
	}
	allowedOrigins := parseOrigins(os.Getenv("WS_ALLOWED_ORIGINS"))
	return &DataReceiver{
		prod: producer,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return originAllowed(r, allowedOrigins)
			},
		},
		readLimit:      readLimit,
		readTimeout:    30 * time.Second,
		produceTimeout: 5 * time.Second,
		connections:    make(map[*websocket.Conn]struct{}),
	}
}

func (dr *DataReceiver) closeConnections() {
	dr.connectionsMu.Lock()
	defer dr.connectionsMu.Unlock()
	for conn := range dr.connections {
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"), time.Now().Add(time.Second))
		_ = conn.Close()
	}
}

func (dr *DataReceiver) wsReceiveLoop(parent context.Context, conn *websocket.Conn) {
	log.Println("new OBU connection")
	conn.SetReadLimit(dr.readLimit)
	_ = conn.SetReadDeadline(time.Now().Add(dr.readTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(dr.readTimeout))
	})
	for {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) || errors.Is(err, context.Canceled) {
				return
			}
			log.Printf("WebSocket read ended: %v", err)
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(dr.readTimeout))
		receiverEvents.Inc()
		var data types.OBUData
		if err := json.Unmarshal(payload, &data); err != nil {
			receiverInvalidEvents.Inc()
			continue
		}
		if data.EventID == "" || data.ProducedAtUnixNano <= 0 || data.OBUID <= 0 {
			receiverInvalidEvents.Inc()
			continue
		}
		ctx, cancel := context.WithTimeout(parent, dr.produceTimeout)
		err = dr.prod.ProduceData(ctx, data)
		cancel()
		if err != nil {
			kafkaEnqueueFailures.Inc()
			log.Printf("Kafka produce failed for event %s: %v", data.EventID, err)
		}
	}
}

func parseOrigins(value string) map[string]struct{} {
	origins := make(map[string]struct{})
	for _, origin := range strings.Split(value, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins[origin] = struct{}{}
		}
	}
	return origins
}

func originAllowed(r *http.Request, allowed map[string]struct{}) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	if _, ok := allowed["*"]; ok {
		return true
	}
	if _, ok := allowed[origin]; ok {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, r.Host)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
