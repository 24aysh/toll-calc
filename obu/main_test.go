package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestParseRamp(t *testing.T) {
	phases, err := parseRamp("30s:100,1m:500")
	if err != nil {
		t.Fatal(err)
	}
	if len(phases) != 2 || phases[0].duration != 30*time.Second || phases[0].rate != 100 || phases[1].duration != time.Minute || phases[1].rate != 500 {
		t.Fatalf("unexpected phases: %+v", phases)
	}
}

func TestEventIdentityIsUnique(t *testing.T) {
	cfg := loadConfig{OBUs: 2, Seed: 7}
	stats := &loadStats{runID: 123}
	first := nextEvent(cfg, stats, rand.New(rand.NewSource(7)), "")
	second := nextEvent(cfg, stats, rand.New(rand.NewSource(7)), "")
	if first.EventID == second.EventID || first.ProducedAtUnixNano <= 0 || second.OBUID != 2 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}
