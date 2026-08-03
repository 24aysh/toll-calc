# Benchmark Design and Calculations

## Scope

The repository runs only two benchmark groups:

1. Direct HTTP versus gRPC aggregation latency.
2. Full GPS event-processing latency at different offered loads.

The output contains only p95 and p99 latency tables. It does not report CPU,
memory, throughput limits, or unrelated microbenchmarks.

Run both groups with:

```bash
./benchmark.sh
```

The script prints the tables and writes them to `benchmark_report.md`.

## What p95 and p99 mean

If 100 request durations are sorted from fastest to slowest:

- p95 is the 95th value. About 95% finished at or below it.
- p99 is the 99th value. About 99% finished at or below it.

These values show slow requests that an average can hide. Lower is better when
the benchmark conditions are the same.

## Test isolation

The script creates a unique Kafka topic and consumer group for each run:

```sh
KAFKA_TOPIC="tollify-benchmark-$(date +%s)-$$"
KAFKA_GROUP_ID="$KAFKA_TOPIC-calculator"
```

It stops the normal OBU container so background events cannot enter the sample.
It starts only Kafka, aggregator, receiver, and calculator. The gateway is not
used in either benchmark.

The calculator uses gRPC during the full-pipeline benchmark unless
`AGGREGATOR_PROTOCOL` is overridden in the environment.

## Benchmark 1: HTTP versus gRPC

### Path under test

```text
benchmark runner → HTTP or gRPC → aggregator → same service → same store
```

WebSocket, Kafka, distance calculation, gateway, and invoice reads are excluded.

### Default workload

| Setting | Value |
|---|---:|
| Calls per protocol | `5,000` |
| Concurrent workers | `20` |
| Warm-up calls per protocol | `100` |
| Per-call deadline | `5 seconds` |
| HTTP endpoint | `POST 127.0.0.1:4000/agg` |
| gRPC endpoint | `127.0.0.1:3001 Aggregator/Aggregate` |

Every call sends an equivalent OBU ID, distance value, event ID, and timestamp.
Unique event IDs prevent duplicate filtering from changing the measured work.

HTTP uses one client and reusable connection pool. gRPC uses one reusable client
connection. Warm-up calls establish connections and initialize code paths before
measurement.

### Measured time

Each worker measures immediately around the send call:

```go
start := time.Now()
err := send(ctx, eventID)
latencies[i] = time.Since(start)
```

For HTTP this includes JSON creation, request/response transfer, aggregator
handling, and response-body reading. For gRPC it includes protobuf handling,
request/response transfer, and aggregator handling.

Any request error fails the benchmark instead of removing a slow or failed
sample.

### Exact percentile calculation

Direct transport durations are sorted. For percentile `q` and `N` samples, the
runner uses nearest rank:

```text
index = ceil(q × N) - 1
percentile = sorted[index]
```

For 5,000 samples:

- p95 uses sorted position `4,750` in one-based terms.
- p99 uses sorted position `4,950` in one-based terms.

## Benchmark 2: end-to-end event latency

### Path under test

```text
benchmark GPS creation
  → WebSocket
  → data receiver
  → Kafka acknowledgement and queue
  → distance calculator
  → gRPC aggregator call by default
  → unique invoice-state update
```

The later Kafka offset commit and invoice GET request are not part of this timer.

### Default workload

| Setting | Value |
|---|---:|
| Warm-up | `50 EPS` for `2 seconds` = `100 events` |
| Load levels | `100`, `500`, `1,000 EPS` |
| Time per load | `5 seconds` |
| Events per load | `500`, `2,500`, `5,000` |
| OBU IDs | `100` |
| WebSocket connections | `10` |

`EPS` means events per second offered to the WebSocket input. It is the requested
send rate, not a separate throughput result.

The sender calculates the planned time of every event and waits until that time.
It fails the load if sending takes over one second longer than the configured
window.

### Timer boundaries

The source timestamp is set immediately before the WebSocket write:

```go
ProducedAtUnixNano: time.Now().UnixNano()
```

After the aggregator applies a new event, it records:

```go
time.Since(time.Unix(0, ProducedAtUnixNano))
```

Duplicate events do not add latency samples.

### Waiting for full processing

The Prometheus histogram is cumulative. Before each load, the runner scrapes its
bucket counts. After sending, it waits until the `+Inf` bucket has increased by
the exact number of events sent.

The maximum drain wait is the greater of 30 seconds and three times the load
duration. A missing or extra sample fails the run.

### Histogram delta

For every bucket boundary, the samples belonging only to the current load are:

```text
load bucket count = after count - before count
```

The histogram starts at 0.25 ms and grows each bucket by a factor of 1.3 for 44
buckets. This covers short local calls and delays up to about 20 seconds.

### Histogram percentile calculation

Let the requested percentile rank be `q × N`. The runner finds the first bucket
whose cumulative count reaches that rank. It estimates a value inside that
bucket with linear interpolation:

```text
estimate = previous_bound
         + (current_bound - previous_bound)
         × (rank - previous_count)
         / (current_count - previous_count)
```

Unlike the direct protocol percentile, this value is an estimate because a
Prometheus bucket stores a count, not every raw duration.

## Configuration overrides

```bash
BENCHMARK_REQUESTS=1000 \
BENCHMARK_CONCURRENCY=10 \
BENCHMARK_LOADS=50,100 \
BENCHMARK_DURATION=3s \
BENCHMARK_REPORT=my_report.md \
./benchmark.sh
```

| Variable | Default | Meaning |
|---|---:|---|
| `BENCHMARK_REQUESTS` | `5000` | Calls for each protocol |
| `BENCHMARK_CONCURRENCY` | `20` | Protocol worker count |
| `BENCHMARK_LOADS` | `100,500,1000` | Pipeline offered EPS values |
| `BENCHMARK_DURATION` | `5s` | Time at each pipeline load |
| `BENCHMARK_REPORT` | `benchmark_report.md` | Output file |

## Current report snapshot

The current `benchmark_report.md` contains this single-run snapshot:

| Protocol | p95 (ms) | p99 (ms) |
|---|---:|---:|
| HTTP | 2.004 | 4.680 |
| gRPC | 2.945 | 5.580 |

| Offered load (EPS) | End-to-end p95 (ms) | End-to-end p99 (ms) |
|---:|---:|---:|
| 100 | 9.717 | 11.484 |
| 500 | 86.839 | 123.838 |
| 1,000 | 9.333 | 12.796 |

These are measurements from one local run, not fixed properties of the code.
The non-monotonic pipeline values show why host load and short test windows can
affect results. Repeat runs and report a median before making a general claim or
using the values outside this project snapshot.
