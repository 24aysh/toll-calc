package main

import (
	"math"

	"github.com/24aysh/toll-calc/types"
)

type CalculatorServicer interface {
	CalculateDist(types.OBUData) (float64, error)
}
type CalcService struct {
	prevPoint []float64
}

func NewCalcService() CalculatorServicer {
	return &CalcService{}
}

func (s *CalcService) CalculateDist(data types.OBUData) (float64, error) {
	dist := 0.0
	if len(s.prevPoint) > 0 {
		dist = calcDist(s.prevPoint[0], s.prevPoint[1], data.Lat, data.Lon)
	}
	s.prevPoint = []float64{data.Lat, data.Lon}
	return dist, nil
}
func calcDist(x1, y1, x2, y2 float64) float64 {
	return math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))
}
