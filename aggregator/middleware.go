package main

import (
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
)

type LogMiddleware struct {
	next Aggregator
}

type MetricsMiddleWare struct {
	reqCounterAgg  prometheus.Counter
	reqCounterCalc prometheus.Counter
	reqLatencyAgg  prometheus.Histogram
	reqLatencyCalc prometheus.Histogram
	next           Aggregator
}

func NewMetricMiddleware(next Aggregator) *MetricsMiddleWare {
	reqCounterAgg := promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "agg",
		Name:      "aggregate_requests_total",
	})

	reqCounterCalc := promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "agg",
		Name:      "calculate_requests_total",
	})

	reqLatencyAgg := promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agg",
		Name:      "aggregate_latency_seconds",
	})

	reqLatencyCalc := promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "agg",
		Name:      "calculate_latency_seconds",
	})

	return &MetricsMiddleWare{
		reqCounterAgg:  reqCounterAgg,
		reqCounterCalc: reqCounterCalc,
		reqLatencyAgg:  reqLatencyAgg,
		reqLatencyCalc: reqLatencyCalc,
		next:           next,
	}
}

func NewLogMiddleware(next Aggregator) Aggregator {
	return &LogMiddleware{
		next: next,
	}
}

func (l *LogMiddleware) AggregateDistance(d types.Distance) (err error) {
	defer func(start time.Time) {
		logrus.WithFields(logrus.Fields{
			"took": time.Since(start),
			"Err":  err,
		}).Info()
	}(time.Now())
	return l.next.AggregateDistance(d)
}

func (l *LogMiddleware) CalculateInvoice(id int) (*types.Invoice, error) {
	defer func(start time.Time) {
		logrus.WithFields(logrus.Fields{
			"Took":                   time.Since(start),
			"Calculated Invoice for": id,
		}).Info()
	}(time.Now())
	return l.next.CalculateInvoice(id)
}

func (m *MetricsMiddleWare) AggregateDistance(d types.Distance) error {
	defer func(start time.Time) {
		m.reqLatencyAgg.Observe(float64(time.Since(start).Seconds()))
		m.reqCounterAgg.Inc()

	}(time.Now())

	return m.next.AggregateDistance(d)
}

func (m *MetricsMiddleWare) CalculateInvoice(id int) (*types.Invoice, error) {
	defer func(start time.Time) {
		m.reqLatencyCalc.Observe(float64(time.Since(start).Seconds()))
		m.reqCounterCalc.Inc()

	}(time.Now())
	return m.next.CalculateInvoice(id)
}

func (m *MetricsMiddleWare) PushMetrics(start time.Time, err error) {

}
