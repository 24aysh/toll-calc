package main

import (
	"errors"
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
	operationDuration prometheus.Histogram
	pipelineDuration  prometheus.Histogram
	eventsApplied     prometheus.Counter
	duplicateEvents   prometheus.Counter
	storeErrors       prometheus.Counter
	next              Aggregator
}

var (
	aggregationOperationDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "aggregation_operation_duration_seconds",
		Buckets: []float64{0.0001, 0.00025, 0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1},
	})
	pipelineEventDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "tollify", Name: "pipeline_event_duration_seconds",
		Buckets: []float64{0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	})
	aggregatorEventsApplied = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "aggregator_events_applied_total",
	})
	aggregatorDuplicateEvents = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "aggregator_duplicate_events_total",
	})
	aggregatorStoreErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "tollify", Name: "aggregator_store_errors_total",
	})
)

func NewMetricMiddleware(next Aggregator) *MetricsMiddleWare {
	return &MetricsMiddleWare{
		operationDuration: aggregationOperationDuration,
		pipelineDuration:  pipelineEventDuration,
		eventsApplied:     aggregatorEventsApplied,
		duplicateEvents:   aggregatorDuplicateEvents,
		storeErrors:       aggregatorStoreErrors,
		next:              next,
	}
}

func NewLogMiddleware(next Aggregator) Aggregator {
	return &LogMiddleware{
		next: next,
	}
}

func (l *LogMiddleware) AggregateDistance(d types.Distance) (applied bool, err error) {
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

func (l *LogMiddleware) Reconciliation() types.EventReconciliation {
	return l.next.Reconciliation()
}

func (m *MetricsMiddleWare) AggregateDistance(d types.Distance) (applied bool, err error) {
	defer func(start time.Time) {
		m.operationDuration.Observe(time.Since(start).Seconds())
		if err != nil {
			if !errors.Is(err, ErrInvalidDistance) {
				m.storeErrors.Inc()
			}
		} else if applied {
			m.eventsApplied.Inc()
			if d.ProducedAtUnixNano > 0 {
				elapsed := time.Since(time.Unix(0, d.ProducedAtUnixNano)).Seconds()
				if elapsed >= 0 {
					m.pipelineDuration.Observe(elapsed)
				}
			}
		} else {
			m.duplicateEvents.Inc()
		}
	}(time.Now())

	return m.next.AggregateDistance(d)
}

func (m *MetricsMiddleWare) CalculateInvoice(id int) (inv *types.Invoice, err error) {
	return m.next.CalculateInvoice(id)
}

func (m *MetricsMiddleWare) Reconciliation() types.EventReconciliation {
	return m.next.Reconciliation()
}
