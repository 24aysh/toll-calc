package main

import (
	"github.com/24aysh/toll-calc/types"
	"github.com/sirupsen/logrus"
)

type LogMiddleware struct {
	next DataProducer
}

func (l *LogMiddleware) ProduceData(data types.OBUData) error {
	logrus.WithFields(logrus.Fields{
		"obuID":     data.OBUID,
		"latitude":  data.Lat,
		"longitude": data.Lon,
	}).Info("Producing -> ")
	return l.next.ProduceData(data)
}

func NewLogMiddleware(next DataProducer) *LogMiddleware {
	return &LogMiddleware{
		next: next,
	}
}
