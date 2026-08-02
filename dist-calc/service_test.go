package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/24aysh/toll-calc/types"
)

func TestCalculateDistTracksEachOBUIndependently(t *testing.T) {
	svc := NewCalcService()
	tests := []struct {
		data types.OBUData
		want float64
	}{
		{types.OBUData{EventID: "a1", OBUID: 1, Lat: 0, Lon: 0}, 0},
		{types.OBUData{EventID: "b1", OBUID: 2, Lat: 100, Lon: 100}, 0},
		{types.OBUData{EventID: "a2", OBUID: 1, Lat: 3, Lon: 4}, 5},
		{types.OBUData{EventID: "b2", OBUID: 2, Lat: 100, Lon: 105}, 5},
	}
	for _, test := range tests {
		got, err := svc.CalculateDist(test.data)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("event %s distance=%v, want %v", test.data.EventID, got, test.want)
		}
	}
}

func TestCalculateDistReplayReturnsOriginalResult(t *testing.T) {
	svc := NewCalcService()
	_, _ = svc.CalculateDist(types.OBUData{EventID: "first", OBUID: 1, Lat: 0, Lon: 0})
	event := types.OBUData{EventID: "second", OBUID: 1, Lat: 3, Lon: 4}
	first, _ := svc.CalculateDist(event)
	replay, _ := svc.CalculateDist(event)
	if first != 5 || replay != first {
		t.Fatalf("first=%v replay=%v, want both 5", first, replay)
	}
}

func TestCalculateDistConcurrentAccess(t *testing.T) {
	svc := NewCalcService()
	var wg sync.WaitGroup
	for obu := 1; obu <= 25; obu++ {
		obu := obu
		wg.Add(1)
		go func() {
			defer wg.Done()
			for point := 0; point < 100; point++ {
				_, err := svc.CalculateDist(types.OBUData{EventID: time.Now().String(), OBUID: obu, Lat: float64(point), Lon: float64(point)})
				if err != nil {
					t.Errorf("calculate: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

type fakeAggregatorClient struct {
	mu       sync.Mutex
	requests []*types.AggregateRequest
	wait     bool
}

func (f *fakeAggregatorClient) Aggregate(ctx context.Context, request *types.AggregateRequest) error {
	if f.wait {
		<-ctx.Done()
		return ctx.Err()
	}
	f.mu.Lock()
	f.requests = append(f.requests, request)
	f.mu.Unlock()
	return nil
}

func (f *fakeAggregatorClient) GetInvoice(context.Context, int) (*types.Invoice, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAggregatorClient) Close() error { return nil }

func TestDownstreamAggregationTimeout(t *testing.T) {
	client := &fakeAggregatorClient{wait: true}
	consumer := &KafkaConsumer{
		aggClient: client, calcService: NewCalcService(), downstreamTimeout: 20 * time.Millisecond,
	}
	err := consumer.processEvent(context.Background(), types.OBUData{
		EventID: "timeout", ProducedAtUnixNano: time.Now().UnixNano(), OBUID: 1,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v, want deadline exceeded", err)
	}
}

func TestEventReplaySendsIdenticalDistance(t *testing.T) {
	client := &fakeAggregatorClient{}
	consumer := &KafkaConsumer{
		aggClient: client, calcService: NewCalcService(), downstreamTimeout: time.Second,
	}
	first := types.OBUData{EventID: "first", ProducedAtUnixNano: time.Now().UnixNano(), OBUID: 1, Lat: 0, Lon: 0}
	second := types.OBUData{EventID: "second", ProducedAtUnixNano: time.Now().UnixNano(), OBUID: 1, Lat: 3, Lon: 4}
	if err := consumer.processEvent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := consumer.processEvent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := consumer.processEvent(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if got := []float64{client.requests[0].Value, client.requests[1].Value, client.requests[2].Value}; got[0] != 0 || got[1] != 5 || got[2] != 5 {
		t.Fatalf("distances=%v, want [0 5 5]", got)
	}
}
