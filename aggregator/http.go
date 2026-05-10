package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/24aysh/toll-calc/types"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func makeHTTPTransport(listenAddr string, svc Aggregator) error {
	fmt.Println("HTTP Transport Running")
	http.HandleFunc("/agg", handleAggregate(svc))
	http.HandleFunc("/invoice", handleGetInvoice(svc))
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(listenAddr, nil)
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
