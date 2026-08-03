package main

import (
	"math"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	values := make([]time.Duration, 100)
	for i := range values {
		values[i] = time.Duration(i+1) * time.Millisecond
	}
	if got := percentile(values, 0.95); got != 95*time.Millisecond {
		t.Fatalf("p95 = %s", got)
	}
	if got := percentile(values, 0.99); got != 99*time.Millisecond {
		t.Fatalf("p99 = %s", got)
	}
}

func TestHistogramQuantile(t *testing.T) {
	values := histogram{0.1: 50, 0.2: 100, math.Inf(1): 100}
	if got := histogramQuantile(values, 0.95); math.Abs(got-0.19) > 0.000001 {
		t.Fatalf("p95 = %f", got)
	}
}

func TestSubtractHistogram(t *testing.T) {
	inf := math.Inf(1)
	delta, err := subtractHistogram(histogram{0.1: 5, inf: 8}, histogram{0.1: 2, inf: 3})
	if err != nil {
		t.Fatal(err)
	}
	if delta[0.1] != 3 || delta[inf] != 5 {
		t.Fatalf("delta = %#v", delta)
	}
}
