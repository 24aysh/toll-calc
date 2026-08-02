package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type loadConfig struct {
	Endpoint     string
	OBUs         int
	Rate         float64
	Connections  int
	Duration     time.Duration
	PayloadBytes int
	Mode         string
	Seed         int64
	Ramp         string
	MetricsAddr  string
}

type loadPhase struct {
	duration time.Duration
	rate     float64
}

type loadStats struct {
	attempted atomic.Uint64
	written   atomic.Uint64
	failed    atomic.Uint64
	sequence  atomic.Uint64
	hashXOR   atomic.Uint64
	hashSum   atomic.Uint64
	runID     int64
}

type summary struct {
	Mode              string  `json:"mode"`
	OBUs              int     `json:"obus"`
	Connections       int     `json:"connections"`
	ConfiguredRate    float64 `json:"configured_rate_events_per_second,omitempty"`
	ElapsedSeconds    float64 `json:"elapsed_seconds"`
	EventsAttempted   uint64  `json:"events_attempted"`
	EventsWritten     uint64  `json:"events_written"`
	WriteFailures     uint64  `json:"write_failures"`
	AchievedWriteRate float64 `json:"achieved_write_rate_events_per_second"`
	EventIDHashXOR    uint64  `json:"event_id_hash_xor"`
	EventIDHashSum    uint64  `json:"event_id_hash_sum"`
}

var (
	obuEventsAttempted = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "obu_events_attempted_total",
	})
	obuEventsWritten = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "obu_events_written_total",
	})
	obuWriteFailures = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "obu_write_failures_total",
	})
	obuActiveConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "tollify", Name: "obu_active_connections",
	})
	obuOfferedRate = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "tollify", Name: "obu_offered_event_rate",
	})
)

func main() {
	cfg := parseFlags()
	if err := validateConfig(cfg); err != nil {
		log.Fatal(err)
	}
	phases, err := parseRamp(cfg.Ramp)
	if err != nil {
		log.Fatal(err)
	}

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx := baseCtx
	var cancel context.CancelFunc = func() {}
	if cfg.Duration > 0 && cfg.Mode == "max" {
		ctx, cancel = context.WithTimeout(baseCtx, cfg.Duration)
	}
	defer cancel()

	metricsServer := startMetricsServer(cfg.MetricsAddr)
	connections, err := dialConnections(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	stats := &loadStats{runID: time.Now().UnixNano()}
	start := time.Now()
	if cfg.Mode == "max" {
		runMaximumThroughput(ctx, cfg, connections, stats)
	} else {
		runFixedRate(ctx, cfg, phases, connections, stats)
	}
	elapsed := time.Since(start)
	closeConnections(connections)
	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = metricsServer.Shutdown(shutdownCtx)
		shutdownCancel()
	}
	printSummary(cfg, stats, elapsed)
}

func parseFlags() loadConfig {
	cfg := loadConfig{}
	flag.StringVar(&cfg.Endpoint, "endpoint", envOr("RECEIVER_WS_ADDR", "ws://127.0.0.1:30000/ws"), "WebSocket receiver endpoint")
	flag.IntVar(&cfg.OBUs, "obus", envInt("OBU_COUNT", 20), "number of simulated OBUs")
	flag.Float64Var(&cfg.Rate, "rate", envFloat("OBU_EVENT_RATE", 4), "total fixed arrival rate in events/second")
	flag.IntVar(&cfg.Connections, "connections", envInt("OBU_CONNECTIONS", 1), "number of WebSocket connections")
	flag.DurationVar(&cfg.Duration, "duration", envDuration("OBU_TEST_DURATION", 0), "test duration; zero runs until interrupted")
	flag.IntVar(&cfg.PayloadBytes, "payload-bytes", envInt("OBU_PAYLOAD_BYTES", 0), "padding bytes added to each event")
	flag.StringVar(&cfg.Mode, "mode", envOr("OBU_LOAD_MODE", "fixed"), "load mode: fixed or max")
	flag.Int64Var(&cfg.Seed, "seed", envInt64("OBU_RANDOM_SEED", 1), "random seed")
	flag.StringVar(&cfg.Ramp, "ramp", envOr("OBU_RAMP_SCHEDULE", ""), "comma-separated duration:rate phases, e.g. 30s:100,1m:500")
	flag.StringVar(&cfg.MetricsAddr, "metrics-addr", envOr("OBU_METRICS_ADDR", ":9100"), "Prometheus listen address; empty disables it")
	flag.Parse()
	cfg.Mode = strings.ToLower(cfg.Mode)
	return cfg
}

func validateConfig(cfg loadConfig) error {
	if cfg.OBUs <= 0 || cfg.Connections <= 0 {
		return errors.New("obus and connections must be positive")
	}
	if cfg.PayloadBytes < 0 {
		return errors.New("payload-bytes cannot be negative")
	}
	if cfg.Mode != "fixed" && cfg.Mode != "max" {
		return fmt.Errorf("mode must be fixed or max, got %q", cfg.Mode)
	}
	if cfg.Mode == "max" && cfg.Ramp != "" {
		return errors.New("ramp schedule is only supported in fixed mode")
	}
	if cfg.Mode == "fixed" && cfg.Rate <= 0 && cfg.Ramp == "" {
		return errors.New("rate must be positive in fixed mode")
	}
	return nil
}

func dialConnections(ctx context.Context, cfg loadConfig) ([]*websocket.Conn, error) {
	connections := make([]*websocket.Conn, 0, cfg.Connections)
	for i := 0; i < cfg.Connections; i++ {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, cfg.Endpoint, nil)
		if err != nil {
			closeConnections(connections)
			return nil, fmt.Errorf("connect WebSocket %d: %w", i, err)
		}
		connections = append(connections, conn)
		obuActiveConnections.Inc()
	}
	return connections, nil
}

func runFixedRate(ctx context.Context, cfg loadConfig, phases []loadPhase, connections []*websocket.Conn, stats *loadStats) {
	jobs := make([]chan types.OBUData, len(connections))
	var workers sync.WaitGroup
	for i, conn := range connections {
		jobs[i] = make(chan types.OBUData, 1024)
		workers.Add(1)
		go writer(ctx, conn, jobs[i], stats, &workers)
	}
	rng := rand.New(rand.NewSource(cfg.Seed))
	padding := strings.Repeat("x", cfg.PayloadBytes)
	if len(phases) == 0 {
		phases = []loadPhase{{duration: cfg.Duration, rate: cfg.Rate}}
	}
	for _, phase := range phases {
		obuOfferedRate.Set(phase.rate)
		interval := time.Duration(float64(time.Second) / phase.rate)
		if interval < time.Nanosecond {
			interval = time.Nanosecond
		}
		ticker := time.NewTicker(interval)
		phaseCtx := ctx
		phaseCancel := func() {}
		if phase.duration > 0 {
			phaseCtx, phaseCancel = context.WithTimeout(ctx, phase.duration)
		}
		phaseDone := false
		for !phaseDone {
			select {
			case <-phaseCtx.Done():
				phaseDone = true
			case <-ticker.C:
				data := nextEvent(cfg, stats, rng, padding)
				stats.attempted.Add(1)
				obuEventsAttempted.Inc()
				connectionIndex := (data.OBUID - 1) % len(jobs)
				select {
				case jobs[connectionIndex] <- data:
				default:
					recordFailure(stats)
				}
			}
		}
		ticker.Stop()
		phaseCancel()
		if ctx.Err() != nil {
			break
		}
	}
	for _, job := range jobs {
		close(job)
	}
	workers.Wait()
}

func runMaximumThroughput(ctx context.Context, cfg loadConfig, connections []*websocket.Conn, stats *loadStats) {
	obuOfferedRate.Set(-1)
	var workers sync.WaitGroup
	padding := strings.Repeat("x", cfg.PayloadBytes)
	for i, conn := range connections {
		assignedOBUs := make([]int, 0, (cfg.OBUs+len(connections)-1)/len(connections))
		for obuID := i + 1; obuID <= cfg.OBUs; obuID += len(connections) {
			assignedOBUs = append(assignedOBUs, obuID)
		}
		if len(assignedOBUs) == 0 {
			continue
		}
		workers.Add(1)
		go func(worker int, ws *websocket.Conn, obuIDs []int) {
			defer workers.Done()
			rng := rand.New(rand.NewSource(cfg.Seed + int64(worker) + 1))
			obuIndex := 0
			for ctx.Err() == nil {
				data := eventForOBU(cfg, stats, rng, padding, obuIDs[obuIndex])
				obuIndex = (obuIndex + 1) % len(obuIDs)
				stats.attempted.Add(1)
				obuEventsAttempted.Inc()
				if err := writeEvent(ws, data); err != nil {
					recordFailure(stats)
					return
				}
				recordSuccess(stats, data.EventID)
			}
		}(i, conn, assignedOBUs)
	}
	workers.Wait()
}

func writer(ctx context.Context, conn *websocket.Conn, jobs <-chan types.OBUData, stats *loadStats, workers *sync.WaitGroup) {
	defer workers.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-jobs:
			if !ok {
				return
			}
			if err := writeEvent(conn, data); err != nil {
				recordFailure(stats)
				return
			}
			recordSuccess(stats, data.EventID)
		}
	}
}

func nextEvent(cfg loadConfig, stats *loadStats, rng *rand.Rand, payload string) types.OBUData {
	sequence := stats.sequence.Load() + 1
	obuID := int((sequence-1)%uint64(cfg.OBUs)) + 1
	return eventForOBU(cfg, stats, rng, payload, obuID)
}

func eventForOBU(cfg loadConfig, stats *loadStats, rng *rand.Rand, payload string, obuID int) types.OBUData {
	sequence := stats.sequence.Add(1)
	return types.OBUData{
		EventID:            fmt.Sprintf("%d-%d-%d", cfg.Seed, stats.runID, sequence),
		ProducedAtUnixNano: time.Now().UnixNano(),
		OBUID:              obuID,
		Lat:                rng.Float64() * 100,
		Lon:                rng.Float64() * 100,
		Payload:            payload,
	}
}

func writeEvent(conn *websocket.Conn, data types.OBUData) error {
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteJSON(data)
}

func recordSuccess(stats *loadStats, eventID string) {
	stats.written.Add(1)
	fingerprint := types.EventIDFingerprint(eventID)
	stats.hashSum.Add(fingerprint)
	for {
		old := stats.hashXOR.Load()
		if stats.hashXOR.CompareAndSwap(old, old^fingerprint) {
			break
		}
	}
	obuEventsWritten.Inc()
}

func recordFailure(stats *loadStats) {
	stats.failed.Add(1)
	obuWriteFailures.Inc()
}

func closeConnections(connections []*websocket.Conn) {
	for _, conn := range connections {
		if conn != nil {
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "test complete"), time.Now().Add(time.Second))
			_ = conn.Close()
			obuActiveConnections.Dec()
		}
	}
}

func parseRamp(value string) ([]loadPhase, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var phases []loadPhase
	for _, raw := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(raw), ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid ramp phase %q; expected duration:rate", raw)
		}
		duration, err := time.ParseDuration(parts[0])
		if err != nil || duration <= 0 {
			return nil, fmt.Errorf("invalid ramp duration %q", parts[0])
		}
		rate, err := strconv.ParseFloat(parts[1], 64)
		if err != nil || rate <= 0 {
			return nil, fmt.Errorf("invalid ramp rate %q", parts[1])
		}
		phases = append(phases, loadPhase{duration: duration, rate: rate})
	}
	return phases, nil
}

func startMetricsServer(addr string) *http.Server {
	if addr == "" {
		return nil
	}
	server := &http.Server{Addr: addr, Handler: promhttp.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server: %v", err)
		}
	}()
	return server
}

func printSummary(cfg loadConfig, stats *loadStats, elapsed time.Duration) {
	written := stats.written.Load()
	result := summary{
		Mode: cfg.Mode, OBUs: cfg.OBUs, Connections: cfg.Connections, ConfiguredRate: cfg.Rate,
		ElapsedSeconds: elapsed.Seconds(), EventsAttempted: stats.attempted.Load(), EventsWritten: written,
		WriteFailures: stats.failed.Load(), AchievedWriteRate: float64(written) / elapsed.Seconds(),
		EventIDHashXOR: stats.hashXOR.Load(), EventIDHashSum: stats.hashSum.Load(),
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(b))
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err == nil {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err == nil {
		return value
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(os.Getenv(key), 64)
	if err == nil {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(key))
	if err == nil {
		return value
	}
	return fallback
}
