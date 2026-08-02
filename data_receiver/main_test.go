package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/24aysh/toll-calc/types"
	"github.com/gorilla/websocket"
)

type memoryProducer struct {
	mu     sync.Mutex
	events map[string]types.OBUData
}

func (p *memoryProducer) ProduceData(_ context.Context, data types.OBUData) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events[data.EventID] = data
	return nil
}

func (p *memoryProducer) Close() error { return nil }

func (p *memoryProducer) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.events)
}

func TestMultipleWebSocketConnectionsRemainIndependent(t *testing.T) {
	producer := &memoryProducer{events: make(map[string]types.OBUData)}
	receiver := NewDataReceiverWithProducer(producer)
	server := httptest.NewServer(newReceiverHandler(receiver))
	defer server.Close()
	endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	connections := make([]*websocket.Conn, 2)
	for i := range connections {
		conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
		if err != nil {
			t.Fatal(err)
		}
		connections[i] = conn
		defer conn.Close()
	}
	var wg sync.WaitGroup
	for i, conn := range connections {
		i, conn := i, conn
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := conn.WriteJSON(types.OBUData{
				EventID: fmt.Sprintf("event-%d", i), ProducedAtUnixNano: time.Now().UnixNano(), OBUID: i + 1,
			})
			if err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}
	wg.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for producer.count() != 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if producer.count() != 2 {
		t.Fatalf("received %d events, want 2", producer.count())
	}
}

func TestKafkaMessageUsesOBUIDAsKey(t *testing.T) {
	message := newKafkaMessage("events", types.OBUData{OBUID: 1234}, []byte("payload"))
	if got := string(message.Key); got != "1234" {
		t.Fatalf("key=%q, want 1234", got)
	}
	if message.TopicPartition.Topic == nil || *message.TopicPartition.Topic != "events" {
		t.Fatalf("unexpected topic: %+v", message.TopicPartition)
	}
}

func newReceiverHandler(receiver *DataReceiver) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", receiver.handleWS)
	return mux
}
