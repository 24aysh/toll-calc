package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
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
	c := client.NewHttpClient("http://localhost:3000") // endpoint of the aggregator svc
	i := &InvoiceHandler{
		Client: c,
	}
	http.HandleFunc("/invoice", makeApiFunc(i.handleGetInvoice))
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func (i *InvoiceHandler) handleGetInvoice(w http.ResponseWriter, r *http.Request) error {
	inv, err := i.Client.GetInvoice(context.Background(), 13)
	if err != nil {
		return err
	}
	return writeJson(w, http.StatusOK, inv)
}

func writeJson(r http.ResponseWriter, status int, v any) error {
	r.WriteHeader(status)
	r.Header().Add("Content-Type", "application/json")
	return json.NewEncoder(r).Encode(v)
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
