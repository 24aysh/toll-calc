package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/24aysh/toll-calc/types"
)

func TestMemoryStoreConcurrentInsertAndRead(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Insert(types.Distance{EventID: "seed", OBUID: 7, Value: 1}); err != nil {
		t.Fatal(err)
	}
	const inserts = 1000
	var wg sync.WaitGroup
	for i := 0; i < inserts; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			applied, err := store.Insert(types.Distance{EventID: fmt.Sprintf("event-%d", i), OBUID: 7, Value: 1})
			if err != nil || !applied {
				t.Errorf("insert %d: applied=%v err=%v", i, applied, err)
			}
		}(i)
		go func() {
			defer wg.Done()
			if _, err := store.Get(7); err != nil {
				t.Errorf("concurrent read: %v", err)
			}
		}()
	}
	wg.Wait()
	got, err := store.Get(7)
	if err != nil {
		t.Fatal(err)
	}
	if want := float64(inserts + 1); got != want {
		t.Fatalf("total = %v, want %v", got, want)
	}
}

func TestMemoryStoreDuplicateEventIsAppliedOnce(t *testing.T) {
	store := NewMemoryStore()
	const workers = 100
	var wg sync.WaitGroup
	var appliedCount int
	var mu sync.Mutex
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			applied, err := store.Insert(types.Distance{EventID: "same-event", OBUID: 9, Value: 5})
			if err != nil {
				t.Errorf("insert: %v", err)
			}
			if applied {
				mu.Lock()
				appliedCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	got, err := store.Get(9)
	if err != nil {
		t.Fatal(err)
	}
	if appliedCount != 1 || got != 5 {
		t.Fatalf("applied=%d total=%v, want applied=1 total=5", appliedCount, got)
	}
}

func TestConsumerRestartReplayDoesNotChangeInvoice(t *testing.T) {
	store := NewMemoryStore()
	event := types.Distance{
		EventID: "replayed-after-restart", ProducedAtUnixNano: time.Now().UnixNano(), OBUID: 11, Value: 8,
	}
	firstProcess := NewInvoiceAggregator(store)
	applied, err := firstProcess.AggregateDistance(event)
	if err != nil || !applied {
		t.Fatalf("first apply: applied=%v err=%v", applied, err)
	}
	// A new service value models a restarted consumer reaching the same store.
	afterRestart := NewInvoiceAggregator(store)
	applied, err = afterRestart.AggregateDistance(event)
	if err != nil || applied {
		t.Fatalf("replay: applied=%v err=%v", applied, err)
	}
	invoice, err := afterRestart.CalculateInvoice(11)
	if err != nil {
		t.Fatal(err)
	}
	if invoice.TotalDist != 8 {
		t.Fatalf("total=%v, want 8", invoice.TotalDist)
	}
	reconciliation := afterRestart.Reconciliation()
	if reconciliation.Count != 1 || reconciliation.XOR != types.EventIDFingerprint(event.EventID) {
		t.Fatalf("reconciliation=%+v", reconciliation)
	}
}

func TestHTTPAndGRPCAggregationShareSemantics(t *testing.T) {
	store := NewMemoryStore()
	svc := NewInvoiceAggregator(store)
	httpDistance := types.Distance{
		EventID: "http-event", ProducedAtUnixNano: time.Now().UnixNano(), OBUID: 42, Value: 2,
	}
	body, err := json.Marshal(httpDistance)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/agg", bytes.NewReader(body))
	newHTTPHandler(svc).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("HTTP status = %d, body=%s", recorder.Code, recorder.Body.String())
	}

	grpcServer := NewGRPCAggregatorServer(svc)
	_, err = grpcServer.Aggregate(context.Background(), &types.AggregateRequest{
		EventID: "grpc-event", ProducedAtUnixNano: time.Now().UnixNano(), ObuID: 42, Value: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	invoice, err := svc.CalculateInvoice(42)
	if err != nil {
		t.Fatal(err)
	}
	if invoice.TotalDist != 5 {
		t.Fatalf("total distance = %v, want 5", invoice.TotalDist)
	}
}
