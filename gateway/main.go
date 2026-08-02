package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/24aysh/toll-calc/aggregator/client"
	"github.com/sirupsen/logrus"
)

type apiFunc func(w http.ResponseWriter, r *http.Request) error

type InvoiceHandler struct {
	client.Client
}

func main() {
	listenAddr := flag.String("ListenAddr", ":6000", "Listen Address for gateway")
	flag.Parse()
	c := client.NewHttpClient(envOr("AGGREGATOR_HTTP_ADDR", "http://localhost:4000"))
	defer c.Close()
	i := &InvoiceHandler{
		Client: c,
	}
	http.HandleFunc("/invoice", makeApiFunc(i.handleGetInvoice))
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func (i *InvoiceHandler) handleGetInvoice(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		return fmt.Errorf("method not allowed")
	}
	id, err := strconv.Atoi(r.URL.Query().Get("obu"))
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid obu query parameter")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	inv, err := i.Client.GetInvoice(ctx, id)
	if err != nil {
		return err
	}
	return writeJson(w, http.StatusOK, inv)
}

func writeJson(r http.ResponseWriter, status int, v any) error {
	r.Header().Set("Content-Type", "application/json")
	r.WriteHeader(status)
	return json.NewEncoder(r).Encode(v)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func makeApiFunc(fn apiFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func(start time.Time) {
			logrus.WithFields(logrus.Fields{
				"Took": time.Since(start),
				"URI":  r.RequestURI,
			}).Info("REQ")
		}(time.Now())

		if err := fn(w, r); err != nil {
			writeJson(w, http.StatusInternalServerError, map[string]string{
				"Error": err.Error(),
			})
		}

	}
}
