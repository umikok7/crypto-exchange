package main

import (
	"fmt"
	"time"

	"github.com/umikok7/crypto-exchange/client"
	"github.com/umikok7/crypto-exchange/server"
)

func main() {
	go server.StartServer()

	time.Sleep(1 * time.Second)

	c := client.NewClient()

	p := client.ParamsPlaceOrderRequest{
		UserId: 8,
		Bid:    true,
		Price:  10_000,
		Size:   1000,
	}
	go func() {
		for {
			resp, err := c.PlaceLimitOrder(&p)
			if err != nil {
				panic(err)
			}
			fmt.Println("bid orderID => ", resp.OrderID)
			time.Sleep(1 * time.Second)
		}
	}()

	askRequest := client.ParamsPlaceOrderRequest{
		UserId: 8,
		Bid:    false,
		Price:  9_000,
		Size:   1000,
	}

	for {
		resp, err := c.PlaceLimitOrder(&askRequest)
		if err != nil {
			panic(err)
		}
		fmt.Println("ask orderID => ", resp.OrderID)

		time.Sleep(1 * time.Second)
	}
	//select {}
}
