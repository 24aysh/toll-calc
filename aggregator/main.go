package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/24aysh/toll-calc/types"
	"google.golang.org/grpc"
)

func main() {
	HttpAddr := flag.String("Httpaddr", ":3000", "The Http address of HTTP")
	GrpcAddr := flag.String("Grpcaddr", ":3001", "The Grpc address of GRPC")
	flag.Parse()
	store := NewMemoryStore()
	svc := NewInvoiceAggregator(store)
	svc = NewLogMiddleware(svc)
	go func() {
		log.Fatal(makeGRPCTransport(*GrpcAddr, svc))
	}()
	time.Sleep(time.Second * 2)
	log.Fatal(makeHTTPTransport(*HttpAddr, svc))

}

func makeHTTPTransport(listenAddr string, svc Aggregator) error {
	http.HandleFunc("/agg", handleAggregate(svc))
	http.HandleFunc("/invoice", handleGetInvoice(svc))
	return http.ListenAndServe(listenAddr, nil)
}

func makeGRPCTransport(listenAddr string, svc Aggregator) error {
	fmt.Println("GRPC Transport Running")
	// make a TCP listener
	l, err := net.Listen("tcp", listenAddr)

	if err != nil {
		return err
	}
	defer l.Close()
	server := grpc.NewServer([]grpc.ServerOption{}...)

	types.RegisterAggregatorServer(server, NewGRPCAggregatorServer(svc))

	return server.Serve(l)

}

func handleGetInvoice(svc Aggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values, ok := r.URL.Query()["obu"]
		if !ok {
			writeJson(w, http.StatusBadRequest, map[string]string{
				"Error": "Missing OBUID",
			})
			return
		}
		obuID, err := strconv.Atoi(values[0])
		if err != nil {
			writeJson(w, http.StatusBadRequest, map[string]string{
				"Error": "Invalid OBU Id",
			})
		}
		invoice, err := svc.CalculateInvoice(obuID)
		if err != nil {
			writeJson(w, http.StatusInternalServerError, map[string]string{
				"Error": err.Error(),
			})
			return
		}

		writeJson(w, http.StatusOK, invoice)
	}

}

func handleAggregate(svc Aggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var dist types.Distance
		if err := json.NewDecoder(r.Body).Decode(&dist); err != nil {

			writeJson(w, http.StatusBadRequest, map[string]string{"Error": err.Error()})
			return
		}
		err := svc.AggregateDistance(dist)
		if err != nil {
			writeJson(w, http.StatusInternalServerError, map[string]string{"Error": err.Error()})
			return
		}
		writeJson(w, http.StatusAccepted, map[string]string{
			"Message": "Success",
		})

	}
}

func writeJson(r http.ResponseWriter, status int, v any) error {
	r.WriteHeader(status)
	r.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(r).Encode(v)

}
