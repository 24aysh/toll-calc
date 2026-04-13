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
	svc = NewLogMiddleware(svc)
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
