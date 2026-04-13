package main

import "github.com/24aysh/toll-calc/types"

type MemoryStore struct {
}

func (m *MemoryStore) Insert(d types.Distance) error {
	return nil
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}
