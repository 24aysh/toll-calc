# OBU Simulator

## Responsibility

The OBU service is a load generator. It creates synthetic GPS events and sends
them to the data receiver over reusable WebSocket connections. It does not
calculate distance or invoices.

Entry point: `obu/main.go`.

## Event creation

Each event gets a run-specific ID, the source timestamp, an OBU ID, coordinates,
and optional padding.

```go
sequence := stats.sequence.Add(1)
return types.OBUData{
    EventID:            fmt.Sprintf("%d-%d-%d", cfg.Seed, stats.runID, sequence),
    ProducedAtUnixNano: time.Now().UnixNano(),
    OBUID:              obuID,
    Lat:                rng.Float64() * 100,
    Lon:                rng.Float64() * 100,
    Payload:            payload,
}
```

`ProducedAtUnixNano` is the start of the end-to-end latency timer. Downstream
services must not replace it.

The coordinates are synthetic numbers from 0 to 100. They are not real GPS
degrees and are not used with a map or road network.

## Load modes

### Fixed-rate mode

Fixed mode creates events at a requested total rate. A ticker controls the
arrival interval:

```go
interval := time.Duration(float64(time.Second) / phase.rate)
ticker := time.NewTicker(interval)
```

Each connection has a writer goroutine and a buffered job channel. An event is
routed by OBU ID so one OBU keeps using the same connection. If the job channel
is full, the attempt is counted as a failure instead of blocking the scheduler.

### Ramp mode

A ramp is a list of `duration:rate` phases. For example:

```text
30s:100,1m:500
```

This means 100 events/second for 30 seconds, followed by 500 events/second for
one minute.

### Maximum-throughput mode

Maximum mode writes as fast as each WebSocket connection allows. OBUs are split
across connections, and each connection runs its own writer loop.

## Connection behavior

The service opens the configured number of WebSocket connections before the
load starts. Every write has a five-second deadline:

```go
func writeEvent(conn *websocket.Conn, data types.OBUData) error {
    _ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
    return conn.WriteJSON(data)
}
```

At the end, it sends a normal WebSocket close message and closes each socket.

## Configuration

Every setting has a command-line flag and an environment variable.

| Flag | Environment variable | Default | Meaning |
|---|---|---:|---|
| `-endpoint` | `RECEIVER_WS_ADDR` | `ws://127.0.0.1:30000/ws` | Receiver WebSocket URL |
| `-obus` | `OBU_COUNT` | `20` | Number of simulated OBUs |
| `-rate` | `OBU_EVENT_RATE` | `4` | Total fixed load in events/second |
| `-connections` | `OBU_CONNECTIONS` | `1` | WebSocket connection count |
| `-duration` | `OBU_TEST_DURATION` | `0` | Run time; zero means until stopped |
| `-payload-bytes` | `OBU_PAYLOAD_BYTES` | `0` | Extra event padding |
| `-mode` | `OBU_LOAD_MODE` | `fixed` | `fixed` or `max` |
| `-seed` | `OBU_RANDOM_SEED` | `1` | Repeatable random seed |
| `-ramp` | `OBU_RAMP_SCHEDULE` | empty | Fixed-rate phases |
| `-metrics-addr` | `OBU_METRICS_ADDR` | `:9100` | Metrics listener |

The service rejects non-positive OBU/connection counts, negative payload size,
unknown modes, and invalid ramp values.

## Result summary and reconciliation

The final JSON summary contains attempted events, successful writes, failures,
elapsed time, and achieved write rate. It also has XOR and sum fingerprints of
all successfully written event IDs.

The aggregator exposes the same values for uniquely applied events. Comparing
count, XOR, and sum is a compact check that the written event set reached the
invoice store.

## Metrics

| Metric | Type | Meaning |
|---|---|---|
| `tollify_obu_events_attempted_total` | Counter | Events offered to writers |
| `tollify_obu_events_written_total` | Counter | Successful WebSocket writes |
| `tollify_obu_write_failures_total` | Counter | Dropped or failed writes |
| `tollify_obu_active_connections` | Gauge | Open WebSocket connections |
| `tollify_obu_offered_event_rate` | Gauge | Requested EPS; `-1` in max mode |

## Shutdown

The process listens for `SIGINT` and `SIGTERM`. It stops generation, waits for
writer goroutines, closes connections, stops its metrics server, and prints the
summary.
