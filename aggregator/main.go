package main

import (
	"flag"
	"log"
)

func main() {
	HttpAddr := flag.String("Httpaddr", ":3000", "The Http address of HTTP")
	GrpcAddr := flag.String("Grpcaddr", ":3001", "The Grpc address of GRPC")
	flag.Parse()
	store := NewMemoryStore()
	svc := NewInvoiceAggregator(store)
	svc = NewMetricMiddleware(svc)
	svc = NewLogMiddleware(svc)
	go func() {
		log.Fatal(makeGRPCTransport(*GrpcAddr, svc))
	}()
	log.Fatal(makeHTTPTransport(*HttpAddr, svc))

}
