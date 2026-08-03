#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

KAFKA_TOPIC="${KAFKA_TOPIC:-tollify-benchmark-$(date +%s)-$$}"
KAFKA_GROUP_ID="${KAFKA_GROUP_ID:-$KAFKA_TOPIC-calculator}"
export KAFKA_TOPIC KAFKA_GROUP_ID

# The normal OBU generator would contaminate the isolated benchmark workload.
docker compose stop obu >/dev/null 2>&1 || true
docker compose up --build --detach --wait broker aggregator receiver calculator >/dev/null

go run ./benchmark \
  -requests "${BENCHMARK_REQUESTS:-5000}" \
  -concurrency "${BENCHMARK_CONCURRENCY:-20}" \
  -duration "${BENCHMARK_DURATION:-5s}" \
  -loads "${BENCHMARK_LOADS:-100,500,1000}" \
  -report "${BENCHMARK_REPORT:-benchmark_report.md}"
