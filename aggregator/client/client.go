package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/24aysh/toll-calc/types"
)

type Client struct {
	Endpoint string
}

func NewClient(endpoint string) *Client {
	return &Client{
		Endpoint: endpoint,
	}
}

func (c *Client) AggregateInvoice(d types.Distance) error {
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.Endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("The Service returned %d", resp.StatusCode)
	}
	return nil
}
