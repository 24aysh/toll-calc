package main

import "github.com/24aysh/toll-calc/types"

type Aggregator interface {
	AggregateDistance(types.Distance) error
}

type Storer interface {
	Insert(types.Distance) error
}

type InvoiceAggregator struct {
	store Storer
}

func (i *InvoiceAggregator) AggregateDistance(dist types.Distance) error {
	return i.store.Insert(dist)

}

func NewInvoiceAggregator(store Storer) Aggregator {
	return &InvoiceAggregator{
		store: store,
	}
}
