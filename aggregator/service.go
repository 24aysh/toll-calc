package main

import (
	"errors"
	"fmt"
	"math"

	"github.com/24aysh/toll-calc/types"
)

const basePrice = 3.15

var ErrInvalidDistance = errors.New("invalid distance event")

type Aggregator interface {
	AggregateDistance(types.Distance) (bool, error)
	CalculateInvoice(int) (*types.Invoice, error)
	Reconciliation() types.EventReconciliation
}

type Storer interface {
	Insert(types.Distance) (bool, error)
	Get(int) (float64, error)
	Reconciliation() types.EventReconciliation
}

type InvoiceAggregator struct {
	store Storer
}

func (i *InvoiceAggregator) AggregateDistance(dist types.Distance) (bool, error) {
	if dist.EventID == "" || dist.ProducedAtUnixNano <= 0 || dist.OBUID <= 0 || dist.Value < 0 || math.IsNaN(dist.Value) || math.IsInf(dist.Value, 0) {
		return false, fmt.Errorf("%w: event_id, source timestamp, OBU ID, and non-negative finite distance are required", ErrInvalidDistance)
	}
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

func (i *InvoiceAggregator) Reconciliation() types.EventReconciliation {
	return i.store.Reconciliation()
}

func NewInvoiceAggregator(store Storer) Aggregator {
	return &InvoiceAggregator{
		store: store,
	}
}
