package main

import (
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/sirupsen/logrus"
)

type LogMiddleware struct {
	next Aggregator
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

func (l *LogMiddleware) GetDistance(id int) (float64, error) {
	defer func(start time.Time) {
		logrus.WithFields(logrus.Fields{
			"Took":              time.Since(start),
			"Fetch dist for id": id,
		}).Info()
	}(time.Now())
	return l.next.GetDistance(id)
}
