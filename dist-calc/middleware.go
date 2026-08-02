package main

import (
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/sirupsen/logrus"
)

type LogMiddleware struct {
	next CalculatorServicer
}

func (l *LogMiddleware) CalculateDist(data types.OBUData) (dist float64, err error) {
	defer func(start time.Time) {
		logrus.WithFields(logrus.Fields{
			"Took":       time.Since(start),
			"Total Dist": dist,
			"Error":      err,
		}).Info("Calculate Distance")
	}(time.Now())
	return l.next.CalculateDist(data)
}

func NewLogMiddleware(next CalculatorServicer) CalculatorServicer {
	return &LogMiddleware{
		next: next,
	}
}
