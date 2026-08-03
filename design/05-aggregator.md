# Aggregator

## Responsibility

The aggregator owns invoice state. It accepts calculated distance updates,
applies each event once, returns invoices, and records the final end-to-end
latency sample.

Entry points: `aggregator/main.go`, `aggregator/service.go`,
`aggregator/store.go`, `aggregator/http.go`, and `aggregator/grpc.go`.

## One service behind two transports

The process starts HTTP on `:4000` and gRPC on `:3001`. Both transports convert
their request into `types.Distance` and call the same `Aggregator` service.

```go
store := NewMemoryStore()
var svc Aggregator = NewInvoiceAggregator(store)
svc = NewMetricMiddleware(svc)
svc = NewLogMiddleware(svc)
```

This is important for the protocol benchmark: HTTP and gRPC use the same
validation, store, locking, duplicate handling, and metrics middleware.

## gRPC contract

The protocol buffer defines one internal method:

```proto
service Aggregator {
    rpc Aggregate(AggregateRequest) returns (None);
}

message AggregateRequest {
    int32 ObuID = 1;
    double Value = 2;
    int64 ProducedAtUnixNano = 3;
    string EventID = 4;
}
```

The server maps invalid distance input to gRPC `InvalidArgument`. Other errors
are returned as server errors.

## HTTP endpoints

| Method and path | Purpose | Success |
|---|---|---:|
| `POST /agg` | Apply a JSON `Distance` update | `202 Accepted` |
| `GET /invoice?obu=ID` | Calculate and return an invoice | `200 OK` |
| `GET /benchmark/reconciliation` | Return applied-event count and fingerprints | `200 OK` |
| `GET /metrics` | Expose Prometheus metrics | `200 OK` |

`POST /agg` limits the request body to 1 MiB. Missing or invalid JSON returns
`400`. Invoice lookup currently returns `500` when no data exists for the OBU.

## Input validation

The service rejects an update when any of these are true:

- Event ID is empty.
- Source timestamp is zero or negative.
- OBU ID is zero or negative.
- Distance is negative, `NaN`, or infinite.

```go
if dist.EventID == "" || dist.ProducedAtUnixNano <= 0 ||
    dist.OBUID <= 0 || dist.Value < 0 ||
    math.IsNaN(dist.Value) || math.IsInf(dist.Value, 0) {
    return false, ErrInvalidDistance
}
```

Zero distance is valid because an OBU's first GPS point has no previous point.

## In-memory store and idempotency

The store uses one write lock for an update. It first checks the event ID, then
adds the distance to the OBU total.

```go
m.mu.Lock()
defer m.mu.Unlock()
if _, ok := m.processed[d.EventID]; ok {
    return false, nil
}
m.processed[d.EventID] = struct{}{}
m.data[d.OBUID] += d.Value
```

This makes duplicate retries safe inside one running aggregator. An `RWMutex`
also allows concurrent invoice reads while protecting maps from data races.

The reconciliation value stores:

- Count of unique applied events.
- XOR of 64-bit FNV-1a event ID hashes.
- Sum of the same hashes.

It checks event-set equality without storing every ID in the benchmark client.

## Invoice calculation

The store returns total distance for one OBU. The service multiplies it by the
fixed price `3.15`:

```go
inv := &types.Invoice{
    OBUID:     id,
    TotalDist: dist,
    Amount:    dist * 3.15,
}
```

For a total distance of `10`, the invoice amount is `31.5`.

## End-to-end timer

The metric middleware runs after a successful store call. Only a newly applied
event produces an end-to-end sample:

```go
if applied {
    elapsed := time.Since(time.Unix(0, d.ProducedAtUnixNano)).Seconds()
    pipelineDuration.Observe(elapsed)
}
```

The timer therefore covers OBU creation, WebSocket transfer, receiver work,
Kafka acknowledgement and queueing, distance calculation, the downstream call,
and the invoice-state update.

## Metrics

| Metric | Meaning |
|---|---|
| `tollify_aggregation_operation_duration_seconds` | Business and store call time |
| `tollify_pipeline_event_duration_seconds` | OBU creation to successful unique apply |
| `tollify_aggregator_events_applied_total` | Unique updates applied |
| `tollify_aggregator_duplicate_events_total` | Duplicate event IDs ignored |
| `tollify_aggregator_store_errors_total` | Store errors other than input validation |
| `tollify_aggregator_events_received_total{protocol}` | HTTP/gRPC update calls received |
| `tollify_aggregator_open_connections{protocol}` | Open HTTP/gRPC connections |
| `tollify_http_server_request_duration_seconds` | HTTP handler duration |
| `tollify_http_requests_total` | HTTP counts by role, operation, and code |
| `tollify_grpc_server_request_duration_seconds` | gRPC method duration |
| `tollify_grpc_requests_total` | gRPC counts by role, method, and code |

## Shutdown and limits

`SIGINT` or `SIGTERM` starts a ten-second graceful shutdown for HTTP and gRPC.

All invoice and duplicate state is in memory. Restarting the aggregator clears
it. Multiple replicas would need a shared database with an atomic distance
update and a unique event-ID constraint.
