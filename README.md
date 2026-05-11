# Toll Calculator

A distributed toll calculation system built in Go using a microservices architecture.

## Architecture

```
OBU Simulator → Data Receiver → Kafka → Distance Calculator → Aggregator → Gateway → Client
                                                   (gRPC)          (HTTP)
```

| Service | Description |
|---|---|
| `obu` | Simulates On-Board Units sending GPS coordinates |
| `data_receiver` | Receives OBU data and publishes to Kafka |
| `dist-calc` | Consumes Kafka events and calculates distance |
| `aggregator` | Aggregates distances and calculates invoices |
| `gateway` | HTTP API gateway for client-facing endpoints |

## Tech Stack

- **Language:** Go
- **Transport:** gRPC (inter-service) + HTTP REST (client-facing)
- **Message Broker:** Apache Kafka
- **Monitoring:** Prometheus (request counters, error rates, latency histograms)
- **Serialization:** Protocol Buffers

## Running Locally

Start Kafka:
```bash
docker-compose up -d
```

Run each service in a separate terminal:
```bash
make aggregator
make calc
make receiver
make gate
make obu
```

## API

```
GET /invoice?obu=<id>    # Get toll invoice for an OBU
```

## Generate Protobuf

```bash
make proto
```