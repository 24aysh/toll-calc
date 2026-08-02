package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpServerDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "http_server_request_duration_seconds",
		Buckets: []float64{0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}, []string{"method", "route"})
	httpServerRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tollify", Name: "http_requests_total",
	}, []string{"role", "method", "operation", "code"})
	aggregatorProtocolEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "tollify", Name: "aggregator_events_received_total",
	}, []string{"protocol"})
	aggregatorOpenConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "tollify", Name: "aggregator_open_connections",
	}, []string{"protocol"})
)

func makeHTTPTransport(listenAddr string, svc Aggregator) error {
	fmt.Println("HTTP Transport Running")
	return http.ListenAndServe(listenAddr, newHTTPHandler(svc))
}

func newHTTPHandler(svc Aggregator) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/agg", instrumentHTTP("aggregate", handleAggregate(svc)))
	mux.Handle("/invoice", instrumentHTTP("invoice", handleGetInvoice(svc)))
	mux.Handle("/benchmark/reconciliation", instrumentHTTP("reconciliation", handleReconciliation(svc)))
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

func handleReconciliation(svc Aggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJson(w, http.StatusMethodNotAllowed, map[string]string{"Error": "method not allowed"})
			return
		}
		writeJson(w, http.StatusOK, svc.Reconciliation())
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func instrumentHTTP(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		httpServerDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		httpServerRequests.WithLabelValues("server", r.Method, route, strconv.Itoa(sw.status)).Inc()
	})
}

func handleGetInvoice(svc Aggregator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJson(w, http.StatusMethodNotAllowed, map[string]string{"Error": "method not allowed"})
			return
		}
		value := r.URL.Query().Get("obu")
		if value == "" {
			writeJson(w, http.StatusBadRequest, map[string]string{
				"Error": "Missing OBUID",
			})
			return
		}
		obuID, err := strconv.Atoi(value)
		if err != nil {
			writeJson(w, http.StatusBadRequest, map[string]string{
				"Error": "Invalid OBU Id",
			})
			return
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
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJson(w, http.StatusMethodNotAllowed, map[string]string{"Error": "method not allowed"})
			return
		}
		aggregatorProtocolEvents.WithLabelValues("http").Inc()
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var dist types.Distance
		if err := json.NewDecoder(r.Body).Decode(&dist); err != nil {

			writeJson(w, http.StatusBadRequest, map[string]string{"Error": err.Error()})
			return
		}
		_, err := svc.AggregateDistance(dist)
		if err != nil {
			if errors.Is(err, ErrInvalidDistance) {
				writeJson(w, http.StatusBadRequest, map[string]string{"Error": err.Error()})
				return
			}
			writeJson(w, http.StatusInternalServerError, map[string]string{"Error": err.Error()})
			return
		}
		writeJson(w, http.StatusAccepted, map[string]string{
			"Message": "Success",
		})

	}
}

func writeJson(r http.ResponseWriter, status int, v any) error {
	r.Header().Set("Content-Type", "application/json")
	r.WriteHeader(status)
	return json.NewEncoder(r).Encode(v)
}
