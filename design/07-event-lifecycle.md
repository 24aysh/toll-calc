# Complete GPS Event and Invoice Lifecycle

This file follows one OBU through two GPS events and then reads its invoice.
Two events are useful because the first point sets a starting position, while
the second point produces a distance.

## Example input

Assume OBU `1` sends these points:

| Event | Coordinates | Distance added |
|---|---|---:|
| `event-1` | `(0, 0)` | `0` |
| `event-2` | `(3, 4)` | `5` |

The final distance is `5`, so the invoice amount is `5 × 3.15 = 15.75`.

## Lifecycle map

```mermaid
sequenceDiagram
    participant OBU as OBU simulator
    participant R as Data receiver
    participant K as Kafka
    participant C as Distance calculator
    participant A as Aggregator
    participant G as API gateway
    participant U as API client

    OBU->>R: WebSocket OBUData event-1 (0,0)
    R->>K: JSON record keyed by OBU 1
    K-->>R: Delivery acknowledgement
    K->>C: Consume event-1
    C->>C: No previous point; distance = 0
    C->>A: gRPC Aggregate(event-1, distance 0)
    A->>A: Apply event once; total = 0
    A-->>C: Success
    C->>K: Commit event-1 offset

    OBU->>R: WebSocket OBUData event-2 (3,4)
    R->>K: JSON record keyed by OBU 1
    K-->>R: Delivery acknowledgement
    K->>C: Consume event-2
    C->>C: sqrt((3-0)^2 + (4-0)^2) = 5
    C->>A: gRPC Aggregate(event-2, distance 5)
    A->>A: Apply event once; total = 5
    A-->>C: Success
    C->>K: Commit event-2 offset

    U->>G: GET /invoice?obu=1
    G->>A: GET /invoice?obu=1
    A->>A: amount = 5 × 3.15
    A-->>G: Invoice JSON
    G-->>U: total_distance=5, amount=15.75
```

## Step 1: OBU creates the source event

The OBU assigns the event ID and source time immediately before sending. For
example:

```json
{
  "event_id": "event-2",
  "produced_at_unix_nano": 1785744000000000000,
  "obu_id": 1,
  "lat": 3,
  "lon": 4
}
```

The timestamp starts the end-to-end timer. The OBU writes this JSON through an
already open WebSocket connection.

## Step 2: Receiver validates and publishes

The data receiver upgrades `/ws`, reads the JSON, and checks event ID, timestamp,
and OBU ID. It serializes the unchanged event into a Kafka record.

The record key is `"1"`, the OBU ID. The receiver waits until Kafka sends a
delivery result because the producer is configured with `acks=all`.

## Step 3: Kafka stores and orders the record

Kafka holds the event in topic `obudata` by default. Events with the same OBU key
go to the same partition, which keeps their order inside that partition.

Kafka separates the write speed of the OBU/receiver from the processing speed of
the calculator. If processing slows down, records remain in Kafka and consumer
lag grows.

## Step 4: Calculator reads and validates

The calculator consumer group receives the record and decodes it into `OBUData`.
It does not commit the valid Kafka record yet.

For `event-1`, there is no previous point for OBU `1`, so the distance is zero
and `(0,0)` becomes the previous point.

For `event-2`, the service reads `(0,0)` and calculates:

```text
sqrt((3 - 0)^2 + (4 - 0)^2) = sqrt(9 + 16) = 5
```

It stores `(3,4)` as the new previous point and caches `event-2 → 5` for replay.

## Step 5: Calculator calls the aggregator

The calculator creates an `AggregateRequest` with the distance, OBU ID, original
event ID, and original source timestamp. The default transport is one reusable
gRPC connection. HTTP can be selected without changing the business fields.

The downstream call has a two-second deadline by default.

## Step 6: Aggregator updates invoice state

The aggregator validates the request and checks whether the event ID was already
applied. For a new event, one lock protects these actions:

1. Mark the event ID as processed.
2. Add the distance to the total for the OBU.
3. Update reconciliation fingerprints.

After the unique update succeeds, the metrics middleware records:

```text
current time - produced_at_unix_nano
```

This is the end of the benchmark's end-to-end timer. The later gRPC reply and
Kafka offset commit are not included in that timer.

## Step 7: Calculator commits Kafka progress

When the aggregator returns success, the calculator commits the Kafka record.
If aggregation fails, the calculator seeks back to the failed offset instead.

If the aggregator applied the event but the reply was lost, the retry sends the
same event ID and distance. The aggregator ignores the duplicate, preventing a
double charge.

## Step 8: Client reads the final invoice

Invoice reading is a separate request path. The client calls the gateway:

```http
GET http://127.0.0.1:6000/invoice?obu=1
```

The gateway validates OBU `1`, applies a two-second deadline, and calls the
aggregator's HTTP invoice endpoint. The aggregator reads total distance `5` and
calculates the amount on demand.

```json
{
  "obu_id": 1,
  "total_distance": 5,
  "amount": 15.75
}
```

The invoice read is not part of event-processing latency. That benchmark stops
when the distance update is safely applied to the in-memory invoice state.
