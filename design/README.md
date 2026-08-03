# Tollify Design Guide

This directory explains how Tollify works. The files are split by topic so a
new reader can learn one part at a time.

## Suggested reading order

1. [System architecture](01-architecture.md)
2. [OBU simulator](02-obu-simulator.md)
3. [Data receiver](03-data-receiver.md)
4. [Distance calculator](04-distance-calculator.md)
5. [Aggregator](05-aggregator.md)
6. [API gateway](06-api-gateway.md)
7. [Complete event lifecycle](07-event-lifecycle.md)
8. [Observability and deployment](08-observability-and-deployment.md)
9. [Benchmark design and calculations](09-benchmarks.md)
10. [ATS-ready résumé bullets](10-resume-bullets.md)

## Main source directories

| Directory | Purpose |
|---|---|
| `obu/` | Creates simulated GPS events and sends them over WebSocket. |
| `data_receiver/` | Validates WebSocket events and writes them to Kafka. |
| `dist-calc/` | Reads Kafka events, calculates distance, and calls the aggregator. |
| `aggregator/` | Stores distance totals and calculates invoices. |
| `gateway/` | Exposes the public invoice REST endpoint. |
| `types/` | Holds shared Go types and the gRPC service definition. |
| `benchmark/` | Runs the HTTP/gRPC and end-to-end latency benchmarks. |

The design describes the current code. It also calls out places where a local
demo design would need stronger storage or error handling before production use.
