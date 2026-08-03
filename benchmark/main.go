package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	httpEndpoint = "http://127.0.0.1:4000"
	grpcEndpoint = "127.0.0.1:3001"
	wsEndpoint   = "ws://127.0.0.1:30000/ws"
	metricsURL   = httpEndpoint + "/metrics"
)

type config struct {
	requests, concurrency int
	duration              time.Duration
	loads                 []int
	report                string
}

type result struct {
	name     string
	p95, p99 time.Duration
}

type histogram map[float64]uint64

func main() {
	cfg, err := parseConfig()
	if err != nil {
		fail(err)
	}
	if err := waitForMetrics(30 * time.Second); err != nil {
		fail(err)
	}

	httpResult, err := benchmarkHTTP(cfg)
	if err != nil {
		fail(err)
	}
	grpcResult, err := benchmarkGRPC(cfg)
	if err != nil {
		fail(err)
	}
	pipeline, err := benchmarkPipeline(cfg)
	if err != nil {
		fail(err)
	}

	report := formatReport([]result{httpResult, grpcResult}, pipeline)
	if err := os.WriteFile(cfg.report, []byte(report), 0o644); err != nil {
		fail(err)
	}
	fmt.Print(report)
}

func parseConfig() (config, error) {
	var cfg config
	var loads string
	flag.IntVar(&cfg.requests, "requests", 5000, "requests per transport")
	flag.IntVar(&cfg.concurrency, "concurrency", 20, "transport concurrency")
	flag.DurationVar(&cfg.duration, "duration", 5*time.Second, "duration per pipeline load")
	flag.StringVar(&loads, "loads", "100,500,1000", "pipeline events/second")
	flag.StringVar(&cfg.report, "report", "benchmark_report.md", "report path")
	flag.Parse()
	if cfg.requests < 100 || cfg.concurrency < 1 || cfg.duration <= 0 {
		return cfg, errors.New("requests must be at least 100; concurrency and duration must be positive")
	}
	for _, text := range strings.Split(loads, ",") {
		load, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || load <= 0 {
			return cfg, fmt.Errorf("invalid pipeline load %q", text)
		}
		cfg.loads = append(cfg.loads, load)
	}
	return cfg, nil
}

func benchmarkHTTP(cfg config) (result, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = cfg.concurrency
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	defer transport.CloseIdleConnections()

	return benchmarkTransport("HTTP", cfg, func(ctx context.Context, id string) error {
		body, _ := json.Marshal(types.Distance{
			EventID: id, ProducedAtUnixNano: time.Now().UnixNano(), OBUID: 1, Value: 1,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, httpEndpoint+"/agg", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("HTTP status %d", resp.StatusCode)
		}
		return nil
	})
}

func benchmarkGRPC(cfg config) (result, error) {
	conn, err := grpc.NewClient(grpcEndpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return result{}, err
	}
	defer conn.Close()
	client := types.NewAggregatorClient(conn)
	return benchmarkTransport("gRPC", cfg, func(ctx context.Context, id string) error {
		_, err := client.Aggregate(ctx, &types.AggregateRequest{
			EventID: id, ProducedAtUnixNano: time.Now().UnixNano(), ObuID: 1, Value: 1,
		})
		return err
	})
}

func benchmarkTransport(name string, cfg config, send func(context.Context, string) error) (result, error) {
	runID := strconv.FormatInt(time.Now().UnixNano(), 10)
	for i := 0; i < 100; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := send(ctx, fmt.Sprintf("warm-%s-%s-%d", name, runID, i))
		cancel()
		if err != nil {
			return result{}, err
		}
	}

	latencies := make([]time.Duration, cfg.requests)
	jobs, errs := make(chan int), make(chan error, cfg.requests)
	var workers sync.WaitGroup
	for worker := 0; worker < cfg.concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				start := time.Now()
				err := send(ctx, fmt.Sprintf("bench-%s-%s-%d", name, runID, i))
				latencies[i] = time.Since(start)
				cancel()
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	for i := 0; i < cfg.requests; i++ {
		jobs <- i
	}
	close(jobs)
	workers.Wait()
	select {
	case err := <-errs:
		return result{}, fmt.Errorf("%s benchmark: %w", name, err)
	default:
	}
	return result{name: name, p95: percentile(latencies, .95), p99: percentile(latencies, .99)}, nil
}

func benchmarkPipeline(cfg config) ([]result, error) {
	// Exclude Kafka topic creation and consumer-group startup.
	before, err := scrapeHistogram()
	if err != nil {
		return nil, err
	}
	if _, err := sendPipelineLoad(50, 2*time.Second, "warm"); err != nil {
		return nil, err
	}
	if _, err = waitForSamples(before[math.Inf(1)]+100, 30*time.Second); err != nil {
		return nil, err
	}

	results := make([]result, 0, len(cfg.loads))
	for _, load := range cfg.loads {
		before, err = scrapeHistogram()
		if err != nil {
			return nil, err
		}
		count, err := sendPipelineLoad(load, cfg.duration, "load")
		if err != nil {
			return nil, fmt.Errorf("%d events/second: %w", load, err)
		}
		after, err := waitForSamples(before[math.Inf(1)]+count, max(30*time.Second, cfg.duration*3))
		if err != nil {
			return nil, err
		}
		delta, err := subtractHistogram(after, before)
		if err != nil || delta[math.Inf(1)] != count {
			return nil, fmt.Errorf("pipeline sample count mismatch at %d events/second", load)
		}
		results = append(results, result{
			name: strconv.Itoa(load),
			p95:  time.Duration(histogramQuantile(delta, .95) * float64(time.Second)),
			p99:  time.Duration(histogramQuantile(delta, .99) * float64(time.Second)),
		})
	}
	return results, nil
}

func sendPipelineLoad(rate int, duration time.Duration, label string) (uint64, error) {
	const connections = 10
	websockets := make([]*websocket.Conn, 0, connections)
	defer func() {
		for _, conn := range websockets {
			_ = conn.Close()
		}
	}()
	for i := 0; i < connections; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsEndpoint, nil)
		if err != nil {
			return 0, err
		}
		websockets = append(websockets, conn)
	}

	total := int(math.Round(float64(rate) * duration.Seconds()))
	runID, start := time.Now().UnixNano(), time.Now()
	for i := 0; i < total; i++ {
		due := start.Add(time.Duration(float64(i) / float64(rate) * float64(time.Second)))
		if delay := time.Until(due); delay > 0 {
			time.Sleep(delay)
		}
		event := types.OBUData{
			EventID: fmt.Sprintf("%s-%d-%d", label, runID, i), ProducedAtUnixNano: time.Now().UnixNano(),
			OBUID: i%100 + 1, Lat: float64(i % 90), Lon: float64((i * 7) % 180),
		}
		conn := websockets[i%connections]
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(event); err != nil {
			return uint64(i), err
		}
	}
	if elapsed := time.Since(start); elapsed > duration+time.Second {
		return uint64(total), fmt.Errorf("sender needed %s for a %s window", elapsed.Round(time.Millisecond), duration)
	}
	return uint64(total), nil
}

func waitForSamples(want uint64, timeout time.Duration) (histogram, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		values, err := scrapeHistogram()
		if err != nil {
			return nil, err
		}
		if values[math.Inf(1)] >= want {
			return values, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("pipeline did not drain before %s", timeout)
}

func scrapeHistogram() (histogram, error) {
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Get(metricsURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	values := make(histogram)
	const prefix = "tollify_pipeline_event_duration_seconds_bucket{le=\""
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		end := strings.Index(line[len(prefix):], "\"")
		fields := strings.Fields(line)
		if end < 0 || len(fields) != 2 {
			continue
		}
		bound, boundErr := strconv.ParseFloat(line[len(prefix):len(prefix)+end], 64)
		count, countErr := strconv.ParseUint(fields[1], 10, 64)
		if boundErr != nil || countErr != nil {
			return nil, errors.New("invalid pipeline histogram")
		}
		values[bound] = count
	}
	if len(values) == 0 {
		return nil, errors.New("pipeline latency histogram not found")
	}
	return values, scanner.Err()
}

func subtractHistogram(after, before histogram) (histogram, error) {
	delta := make(histogram, len(after))
	for bound, count := range after {
		if count < before[bound] {
			return nil, errors.New("pipeline histogram reset during benchmark")
		}
		delta[bound] = count - before[bound]
	}
	return delta, nil
}

func histogramQuantile(values histogram, quantile float64) float64 {
	bounds := make([]float64, 0, len(values))
	for bound := range values {
		bounds = append(bounds, bound)
	}
	sort.Float64s(bounds)
	rank := quantile * float64(values[math.Inf(1)])
	var previousBound, previousCount float64
	for _, bound := range bounds {
		count := float64(values[bound])
		if count >= rank {
			if math.IsInf(bound, 1) {
				return previousBound
			}
			return previousBound + (bound-previousBound)*(rank-previousCount)/(count-previousCount)
		}
		previousBound, previousCount = bound, count
	}
	return 0
}

func percentile(values []time.Duration, quantile float64) time.Duration {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[int(math.Ceil(quantile*float64(len(values))))-1]
}

func formatReport(transports, pipeline []result) string {
	var report strings.Builder
	report.WriteString("| protocol | p95_ms | p99_ms |\n|---|---:|---:|\n")
	for _, value := range transports {
		fmt.Fprintf(&report, "| %s | %.3f | %.3f |\n", value.name, float64(value.p95)/float64(time.Millisecond), float64(value.p99)/float64(time.Millisecond))
	}
	report.WriteString("\n| load_eps | p95_ms | p99_ms |\n|---:|---:|---:|\n")
	for _, value := range pipeline {
		fmt.Fprintf(&report, "| %s | %.3f | %.3f |\n", value.name, float64(value.p95)/float64(time.Millisecond), float64(value.p99)/float64(time.Millisecond))
	}
	return report.String()
}

func waitForMetrics(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := (&http.Client{Timeout: time.Second}).Get(metricsURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("aggregator did not become ready")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
