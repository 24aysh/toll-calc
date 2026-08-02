package main

import (
	"math"
	"sync"

	"github.com/24aysh/toll-calc/types"
)

type CalculatorServicer interface {
	CalculateDist(types.OBUData) (float64, error)
}

type Point struct {
	Lat float64
	Lon float64
}

type CalcService struct {
	mu         sync.Mutex
	prevPoints map[int]Point
	results    map[string]float64
}

func NewCalcService() CalculatorServicer {
	return &CalcService{
		prevPoints: make(map[int]Point),
		results:    make(map[string]float64),
	}
}

func (s *CalcService) CalculateDist(data types.OBUData) (float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if data.EventID != "" {
		if dist, ok := s.results[data.EventID]; ok {
			return dist, nil
		}
	}
	dist := 0.0
	if previous, ok := s.prevPoints[data.OBUID]; ok {
		dist = calcDist(previous.Lat, previous.Lon, data.Lat, data.Lon)
	}
	s.prevPoints[data.OBUID] = Point{Lat: data.Lat, Lon: data.Lon}
	if data.EventID != "" {
		s.results[data.EventID] = dist
	}
	return dist, nil
}
func calcDist(x1, y1, x2, y2 float64) float64 {
	return math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))
}
