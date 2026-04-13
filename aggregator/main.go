package main

import (
	"encoding/json"
	"flag"
	"net/http"

	"github.com/24aysh/toll-calc/types"
)

func main() {
	listenAddr := flag.String("listenaddr", ":3000", "The listen address of HTTP")
	flag.Parse()
	store := NewMemoryStore()
	svc := NewInvoiceAggregator(store)
	makeHTTPTransport(*listenAddr, svc)

}

func makeHTTPTransport(listenAddr string, svc Aggregator) {
	http.HandleFunc("/agg", handleAggregate(svc))
	http.ListenAndServe(listenAddr, nil)
}

func handleAggregate(svc Aggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var dist types.Distance
		if err := json.NewDecoder(r.Body).Decode(&dist); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		err := svc.AggregateDistance(dist)
		resp := []byte("ayush")
		if err != nil {
			w.Write(resp)
		}

	}
}
