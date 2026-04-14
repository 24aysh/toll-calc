package main

import (
	"fmt"

	"github.com/24aysh/toll-calc/types"
)

type MemoryStore struct {
	data map[int]float64
}

func (m *MemoryStore) Insert(d types.Distance) error {
	m.data[d.OBUID] += d.Value
	return nil
}
func (m *MemoryStore) Get(id int) (float64, error) {
	dist, ok := m.data[id]
	if !ok {
		return float64(0.0), fmt.Errorf("Data not found for %d", id)

	}
	return dist, nil
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[int]float64),
	}
}
