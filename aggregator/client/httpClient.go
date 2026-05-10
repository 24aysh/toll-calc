package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/24aysh/toll-calc/types"
)

type HttpClient struct {
	Endpoint string
}

func NewHttpClient(endpoint string) *HttpClient {
	return &HttpClient{
		Endpoint: endpoint,
	}
}

func (c *HttpClient) Aggregate(ctx context.Context, req *types.AggregateRequest) error {
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	r, err := http.NewRequest("POST", c.Endpoint+"/agg", bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("The Service returned %d", resp.StatusCode)
	}
	return nil
}

func (c *HttpClient) GetInvoice(ctx context.Context, id int) (*types.Invoice, error) {
	invReq := types.GetInvoiceRequest{
		ObuId: int32(id),
	}
	b, err := json.Marshal(&invReq)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/%s?obu=%d", c.Endpoint, "invoice", id)

	r, err := http.NewRequest("POST", endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	var inv types.Invoice
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return &inv, nil
}
