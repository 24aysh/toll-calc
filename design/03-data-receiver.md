# Data Receiver

## Responsibility

The data receiver accepts OBU events over WebSocket, checks their required
fields, and publishes valid events to Kafka. It keeps the event ID and source
timestamp unchanged.

Entry points: `data_receiver/main.go` and `data_receiver/producer.go`.

## HTTP endpoints

| Path | Purpose |
|---|---|
| `/ws` | Upgrades an HTTP connection to WebSocket and accepts GPS events. |
| `/metrics` | Exposes Prometheus metrics. |

The default listener is `:30000`, set by `RECEIVER_HTTP_ADDR`.

## WebSocket setup

The service uses 4 KiB read and write buffers. It tracks every open connection
so shutdown can close them cleanly.

```go
upgrader: websocket.Upgrader{
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
    CheckOrigin: func(r *http.Request) bool {
        return originAllowed(r, allowedOrigins)
    },
}
```

The default message limit is 1 MiB. `WS_READ_LIMIT_BYTES` can replace it with a
positive integer. The read deadline is 30 seconds and is refreshed after each
message or pong.

## Origin checks

A request is accepted when any of these rules match:

- It has no `Origin` header, as with most service clients.
- `WS_ALLOWED_ORIGINS` contains `*`.
- Its exact origin is in the comma-separated allowlist.
- Its origin host matches the receiver request host.

## Event validation

The receiver reads one complete WebSocket message, decodes JSON into `OBUData`,
and requires:

- A non-empty event ID.
- A positive source timestamp.
- A positive OBU ID.

Invalid JSON or missing required values are counted and skipped. The receiver
does not change coordinates, the event ID, or the timestamp.

## Kafka publishing

The producer serializes the same `OBUData` value as JSON. It uses the OBU ID as
the Kafka key and lets Kafka choose a partition.

```go
return &kafka.Message{
    TopicPartition: kafka.TopicPartition{
        Topic: &topic,
        Partition: kafka.PartitionAny,
    },
    Key:   []byte(strconv.Itoa(data.OBUID)),
    Value: value,
}
```

The default broker is `localhost:9092`, and the default topic is `obudata`.
`KAFKA_BROKERS` and `KAFKA_TOPIC` change them.

The producer uses `acks=all`. `ProduceData` waits for Kafka's delivery event, so
a WebSocket read is considered published only after Kafka reports the result.
This wait has a five-second timeout.

```go
select {
case <-ctx.Done():
    return ctx.Err()
case event := <-delivery:
    return event.(*kafka.Message).TopicPartition.Error
}
```

Each WebSocket connection has its own receive loop. A slow Kafka acknowledgement
blocks only that connection's loop, not every connected OBU.

## Metrics

| Metric | Type | Meaning |
|---|---|---|
| `tollify_receiver_active_websocket_connections` | Gauge | Current WebSocket connections |
| `tollify_receiver_events_received_total` | Counter | WebSocket messages read |
| `tollify_receiver_invalid_events_total` | Counter | Invalid JSON or missing required values |
| `tollify_receiver_kafka_enqueue_failures_total` | Counter | Events Kafka did not accept or acknowledge |
| `tollify_receiver_kafka_acknowledgement_duration_seconds` | Histogram | Produce call to Kafka delivery result |

The logging middleware also records OBU ID and coordinates before publishing.

## Shutdown

On `SIGINT` or `SIGTERM`, the HTTP server stops accepting work, active sockets
receive a going-away close message, and the Kafka producer flushes for up to five
seconds. The producer reports an error if messages remain undelivered.
