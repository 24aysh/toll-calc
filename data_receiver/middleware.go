package main

import (
	"context"

	"github.com/24aysh/toll-calc/types"
	"github.com/sirupsen/logrus"
)

type LogMiddleware struct {
	next DataProducer
}

func (l *LogMiddleware) ProduceData(ctx context.Context, data types.OBUData) error {
	logrus.WithFields(logrus.Fields{
		"obuID":     data.OBUID,
		"latitude":  data.Lat,
		"longitude": data.Lon,
	}).Info("Producing -> ")
	return l.next.ProduceData(ctx, data)
}

func (l *LogMiddleware) Close() error { return l.next.Close() }

func NewLogMiddleware(next DataProducer) *LogMiddleware {
	return &LogMiddleware{
		next: next,
	}
}
