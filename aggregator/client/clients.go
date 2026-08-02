package client

import (
	"context"

	"github.com/24aysh/toll-calc/types"
)

type Client interface {
	Aggregate(context.Context, *types.AggregateRequest) error
	GetInvoice(context.Context, int) (*types.Invoice, error)
	Close() error
}
