# API Gateway

## Responsibility

The gateway is the public read entry point. It validates an invoice request,
calls the aggregator over HTTP, and returns invoice JSON. It does not receive
GPS events and does not calculate distance.

Entry point: `gateway/main.go`.

## Public endpoint

```http
GET /invoice?obu=1
```

The `obu` query value must be a positive integer. The handler creates a
two-second request context before calling the aggregator.

```go
id, err := strconv.Atoi(r.URL.Query().Get("obu"))
if err != nil || id <= 0 {
    return fmt.Errorf("invalid obu query parameter")
}
ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
defer cancel()
inv, err := i.Client.GetInvoice(ctx, id)
```

## Aggregator call

The gateway always uses the shared HTTP client. Its default aggregator address
is `http://localhost:4000`; `AGGREGATOR_HTTP_ADDR` changes it.

The client calls:

```http
GET /invoice?obu=1
Accept: application/json
```

It reuses an HTTP connection pool with up to 100 idle connections per host and
an idle timeout of 90 seconds. The client drains and closes every response body
so the transport can reuse the connection.

The gRPC client does not implement invoice lookup. gRPC is only used for
distance aggregation in the current design.

## Response

A successful response has this shape:

```json
{
  "obu_id": 1,
  "total_distance": 10,
  "amount": 31.5
}
```

The gateway sets `Content-Type: application/json` and forwards the decoded
invoice value.

## Current error behavior

The handler checks the HTTP method and query value, but the common wrapper
currently turns any returned error into `500 Internal Server Error`. This means
an invalid method, invalid query, timeout, and missing invoice all appear as
server errors to the public client.

A production API would normally map these cases to separate status codes such
as `405`, `400`, `404`, and `504`.

## Logging and configuration

Every request log contains its URI and total gateway handler time. The gateway
does not expose its own Prometheus endpoint in the current code, but its shared
HTTP client registers HTTP client metrics in the process.

| Setting | Default | Meaning |
|---|---|---|
| `-ListenAddr` | `:6000` | Public HTTP listener |
| `AGGREGATOR_HTTP_ADDR` | `http://localhost:4000` | Aggregator target |

The gateway uses `http.ListenAndServe` and stops when the process is terminated;
it does not currently implement a separate graceful-shutdown block.
