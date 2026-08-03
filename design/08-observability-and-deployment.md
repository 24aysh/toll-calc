# Observability and Deployment

## Docker image

All Go services use the same multi-stage `Dockerfile`. The build stage selects a
service directory through the `SERVICE` build argument.

```dockerfile
FROM golang:1.26-bookworm AS builder
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/tollify "./${SERVICE}"
```

The runtime stage uses Debian Bookworm Slim and runs as non-root user `tollify`
with UID `10001`. Each image has one `/usr/local/bin/tollify` entry point.

## Docker Compose services

`docker-compose.yml` builds each Go service independently and connects them by
Compose service name.

| Service | Container dependency | Host ports |
|---|---|---|
| `broker` | None | `127.0.0.1:9092` |
| `aggregator` | None | `127.0.0.1:4000`, `127.0.0.1:3001` |
| `receiver` | Healthy broker | `127.0.0.1:30000` |
| `calculator` | Healthy broker, started aggregator | `127.0.0.1:9102` |
| `gateway` | Started aggregator | `127.0.0.1:6000` |
| `obu` | Started receiver | `127.0.0.1:9100` |
| `prometheus` | Metric-producing services | `127.0.0.1:9090` |
| `grafana` | Prometheus | `127.0.0.1:3000` |

Ports bind to `127.0.0.1`, so they are reachable from the local machine but not
published on every network interface.

The broker health check runs Kafka's topic-list command. Receiver and calculator
startup wait for that check.

## Stored Docker data

| Volume | Contents |
|---|---|
| `kafka_data` | Kafka log data |
| `prometheus_data` | Collected time series |
| `grafana_data` | Grafana state |

Invoice totals, duplicate IDs, calculator points, and cached distances are not
in Docker volumes. They are in process memory.

## Prometheus collection

Prometheus scrapes every five seconds:

```yaml
global:
  scrape_interval: 5s

scrape_configs:
  - job_name: tollify-aggregator
    static_configs:
      - targets: ["aggregator:4000"]
```

The full configuration also scrapes receiver `:30000`, calculator `:9102`, and
OBU `:9100`.

The Go Prometheus client automatically adds process and Go runtime metrics. These
include CPU time, resident memory, goroutines, garbage collection, and file
descriptors where the platform supports them.

## Metric layers

The latency metrics answer different questions:

| Layer | Example metric | Timer boundaries |
|---|---|---|
| Source | `tollify_obu_events_written_total` | OBU write outcome |
| Receiver/Kafka | `tollify_receiver_kafka_acknowledgement_duration_seconds` | Kafka produce call to delivery result |
| Kafka queue | `tollify_kafka_consumer_lag` | Records behind consumed offset |
| Calculation | `tollify_distance_calculation_duration_seconds` | Distance function only |
| Downstream client | `tollify_downstream_aggregation_duration_seconds` | Calculator to aggregator round trip |
| Aggregator business work | `tollify_aggregation_operation_duration_seconds` | Validation and in-memory store call |
| Full event path | `tollify_pipeline_event_duration_seconds` | OBU creation to unique invoice-state update |

HTTP and gRPC also have separate client/server duration histograms and request
counters. This makes it possible to compare the protocols without mixing in
Kafka or WebSocket time.

## Grafana

Grafana is provisioned with Prometheus as its data source. The local default is:

```text
URL:      http://127.0.0.1:3000
Username: admin
Password: admin
```

The repository provisions the data source but does not include fixed dashboards.
Use Grafana Explore or Prometheus queries to inspect metrics.

## Common commands

Start the complete stack:

```bash
make bootstrap
```

Inspect or stop it:

```bash
make bootstrap-status
make bootstrap-logs
make bootstrap-down
```

Remove containers and all three named data volumes:

```bash
make bootstrap-reset
```

Run services natively after starting Kafka:

```bash
docker compose up -d broker
make aggregator
make receiver
make calc
make gate
make obu
```

## Correctness checks

Before publishing benchmark numbers, run:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Do not run a race-enabled binary during latency measurement. Race detection adds
large timing overhead and changes the result.
