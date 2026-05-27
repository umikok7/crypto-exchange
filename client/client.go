package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/umikok7/crypto-exchange/server"
)

const EndPoint = "http://127.0.0.1:3000/"

type ParamsPlaceOrderRequest struct {
	UserId int64
	Bid    bool
	Price  float64
	Size   float64
}

type Client struct {
	Client *http.Client
}

func NewClient() *Client {
	return &Client{
		Client: http.DefaultClient,
	}
}

func (c *Client) PlaceLimitOrder(p *ParamsPlaceOrderRequest) (*server.HandlePlaceOrderResp, error) {
	params := server.PlaceOrderRequest{
		UserId: p.UserId,
		Type:   server.LimitOrder,
		Bid:    p.Bid,
		Size:   p.Size,
		Price:  p.Price,
		Market: server.MarketETH,
	}
	body, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	e := EndPoint + "order"
	req, err := http.NewRequest("POST", e, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("place limit order failed: %s: %s", resp.Status, string(respBody))
	}

	placeLimitOrderResp := &server.HandlePlaceOrderResp{}
	if err := json.Unmarshal(respBody, placeLimitOrderResp); err != nil {
		return nil, err
	}

	return placeLimitOrderResp, nil
}
