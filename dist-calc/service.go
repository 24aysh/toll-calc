package main

import (
	"math"

	"github.com/24aysh/toll-calc/types"
)

type CalculatorServicer interface {
	CalculateDist(types.OBUData) (float64, error)
}
type CalcService struct {
	points [][]float64
}

func NewCalcService() CalculatorServicer {
	return &CalcService{
		points: make([][]float64, 0),
	}
}

func (s *CalcService) CalculateDist(data types.OBUData) (float64, error) {
	dist := 0.0
	if len(s.points) > 0 {
		prevPoint := s.points[len(s.points)-1]
		dist = calcDist(prevPoint[0], prevPoint[1], data.Lat, data.Lon)
	}
	s.points = append(s.points, []float64{data.Lat, data.Lon})
	return dist, nil
}
func calcDist(x1, y1, x2, y2 float64) float64 {
	return math.Sqrt(math.Pow(x2-x1, 2) + math.Pow(y2-y1, 2))
}
