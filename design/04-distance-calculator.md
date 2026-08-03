# Distance Calculator

## Responsibility

The distance calculator consumes GPS events from Kafka, calculates the distance
from the previous point of the same OBU, and sends the result to the aggregator.

Entry points: `dist-calc/main.go`, `dist-calc/consumer.go`, and
`dist-calc/service.go`.

## Kafka consumer setup

The consumer uses these important settings:

```go
kafka.NewConsumer(&kafka.ConfigMap{
    "bootstrap.servers":  cfg.Brokers,
    "group.id":           cfg.GroupID,
    "auto.offset.reset":  "earliest",
    "enable.auto.commit": false,
})
```

Defaults are broker `localhost:9092`, topic `obudata`, and group
`tollify-distance-calculator`. The read loop polls every 250 ms. A poll timeout
is normal and simply starts the next poll.

## Record validation and commit rule

Kafka JSON is decoded into `OBUData`. A record with invalid JSON or a missing
event ID, timestamp, or OBU ID is counted as invalid and committed so one bad
record does not block the partition.

A valid record follows a stricter rule:

```go
if err := c.processEvent(parent, data); err != nil {
    return err
}
_, err := c.consumer.CommitMessage(msg)
```

The offset is committed only after distance calculation and aggregation both
succeed. On a processing error, the consumer seeks back to the failed offset.
This makes the record available for another attempt.

## Distance state and formula

The calculator keeps the last point for each OBU and a result for each event ID.
A mutex protects both maps.

```go
dist := 0.0
if previous, ok := s.prevPoints[data.OBUID]; ok {
    dist = calcDist(previous.Lat, previous.Lon, data.Lat, data.Lon)
}
s.prevPoints[data.OBUID] = Point{Lat: data.Lat, Lon: data.Lon}
s.results[data.EventID] = dist
```

The first point for an OBU adds zero distance because no earlier point exists.
Later points use straight-line Euclidean distance:

```go
func calcDist(x1, y1, x2, y2 float64) float64 {
    return math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))
}
```

For points `(0,0)` and `(3,4)`, the result is `5`. This is a synthetic Cartesian
calculation, not Haversine distance on the Earth.

If Kafka repeats an event ID, the cached result is returned. The point is not
advanced a second time, so the repeated aggregation request carries the same
distance.

## Aggregator protocol switch

`AGGREGATOR_PROTOCOL` selects `grpc` or `http`; gRPC is the default.

```go
switch protocol {
case "http":
    ac = c.NewHttpClientWithTimeout(httpAddress, timeout)
case "grpc":
    ac, err = c.NewGrpcClientWithTimeout(grpcAddress, timeout)
}
```

Both clients implement the same `Client` interface and send the same fields.
Both reuse their connection. The default call timeout is two seconds and comes
from `AGGREGATOR_REQUEST_TIMEOUT`.

The calculator passes the original event identity and timestamp:

```go
req := &types.AggregateRequest{
    Value:              dist,
    ObuID:              int32(data.OBUID),
    EventID:            data.EventID,
    ProducedAtUnixNano: data.ProducedAtUnixNano,
}
```

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `KAFKA_BROKERS` | `localhost:9092` | Kafka address |
| `KAFKA_TOPIC` | `obudata` | Input topic |
| `KAFKA_GROUP_ID` | `tollify-distance-calculator` | Consumer group |
| `AGGREGATOR_PROTOCOL` | `grpc` | Downstream protocol |
| `AGGREGATOR_HTTP_ADDR` | `http://localhost:4000` | HTTP target |
| `AGGREGATOR_GRPC_ADDR` | `localhost:3001` | gRPC target |
| `AGGREGATOR_REQUEST_TIMEOUT` | `2s` | Downstream deadline |
| `CALCULATOR_METRICS_ADDR` | `:9102` | Metrics listener |

## Metrics

| Metric | Type | Meaning |
|---|---|---|
| `tollify_calculator_events_consumed_total` | Counter | Kafka records received |
| `tollify_calculator_consumer_errors_total{stage}` | Counter | Read, process, or seek errors |
| `tollify_calculator_deserialization_errors_total` | Counter | Invalid Kafka records |
| `tollify_distance_calculation_duration_seconds` | Histogram | Distance calculation time |
| `tollify_downstream_aggregation_duration_seconds` | Histogram | Complete aggregator client call |
| `tollify_downstream_timeouts_total` | Counter | Aggregator deadline failures |
| `tollify_kafka_consumer_lag{topic,partition}` | Gauge | Latest known records waiting behind the current offset |

The shared HTTP or gRPC client also records protocol-specific client duration and
status counters.

## Current scaling limit

One consumer loop processes one record at a time. The service state is local to
one process. Running multiple replicas would require a clear plan for shared or
partition-owned previous-point state.
