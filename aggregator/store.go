package main

import (
	"fmt"
	"sync"

	"github.com/24aysh/toll-calc/types"
)

type MemoryStore struct {
	mu        sync.RWMutex
	data      map[int]float64
	processed map[string]struct{}
	reconcile types.EventReconciliation
}

func (m *MemoryStore) Insert(d types.Distance) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d.EventID != "" {
		if _, ok := m.processed[d.EventID]; ok {
			return false, nil
		}
		m.processed[d.EventID] = struct{}{}
		fingerprint := types.EventIDFingerprint(d.EventID)
		m.reconcile.Count++
		m.reconcile.XOR ^= fingerprint
		m.reconcile.Sum += fingerprint
	}
	m.data[d.OBUID] += d.Value
	return true, nil
}

func (m *MemoryStore) Reconciliation() types.EventReconciliation {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reconcile
}

func (m *MemoryStore) Get(id int) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	dist, ok := m.data[id]
	if !ok {
		return float64(0.0), fmt.Errorf("Data not found for %d", id)

	}
	return dist, nil
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data:      make(map[int]float64),
		processed: make(map[string]struct{}),
	}
}
