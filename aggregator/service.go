package main

import "github.com/24aysh/toll-calc/types"

type Aggregator interface {
	AggregateDistance(types.Distance) error
	GetDistance(int) (float64, error)
}

type Storer interface {
	Insert(types.Distance) error
	Get(int) (float64, error)
}

type InvoiceAggregator struct {
	store Storer
}

func (i *InvoiceAggregator) AggregateDistance(dist types.Distance) error {
	return i.store.Insert(dist)

}

func (i *InvoiceAggregator) GetDistance(id int) (float64, error) {
	return i.store.Get(id)
}

func NewInvoiceAggregator(store Storer) Aggregator {
	return &InvoiceAggregator{
		store: store,
	}
}
