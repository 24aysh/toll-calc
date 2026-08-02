FROM golang:1.26-bookworm AS builder

ARG SERVICE
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/tollify "./${SERVICE}"

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 tollify

COPY --from=builder /out/tollify /usr/local/bin/tollify

USER tollify
ENTRYPOINT ["/usr/local/bin/tollify"]
