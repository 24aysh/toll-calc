package main

import "github.com/24aysh/toll-calc/types"

const basePrice = 3.15

type Aggregator interface {
	AggregateDistance(types.Distance) error
	CalculateInvoice(int) (*types.Invoice, error)
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

func (i *InvoiceAggregator) CalculateInvoice(id int) (*types.Invoice, error) {
	dist, err := i.store.Get(id)
	if err != nil {
		return nil, err
	}
	inv := &types.Invoice{
		OBUID:     id,
		TotalDist: dist,
		Amount:    dist * basePrice,
	}
	return inv, nil
}

func NewInvoiceAggregator(store Storer) Aggregator {
	return &InvoiceAggregator{
		store: store,
	}
}
