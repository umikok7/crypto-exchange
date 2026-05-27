package server

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/labstack/echo/v5"
	"github.com/umikok7/crypto-exchange/orderbook"
	"github.com/umikok7/crypto-exchange/utils"
)

const (
	// exchangePrivateKet 是交易所账户的私钥。
	// 当前原型里，用户下限价单时会把 ETH 从用户账户转到这个交易所账户。
	exchangePrivateKet = "4f3edf983ac636a65a842ce7c78d9aa706d3b113bce9c46f30d7d21715b23b1d"

	MarketOrder OrderType = "MARKET"
	LimitOrder  OrderType = "LIMIT"

	MarketETH Market = "ETH"
)

type (
	OrderType string
	Market    string

	PlaceOrderRequest struct {
		UserId int64
		Type   OrderType // limit or market
		Bid    bool
		Size   float64
		Price  float64
		Market Market
	}

	Order struct {
		UserID    int64
		ID        int64
		Price     float64
		Size      float64
		Bid       bool
		TimeStamp int64
	}

	OrderbookData struct {
		TotalAskVolume float64
		TotalBidVolume float64
		Asks           []*Order
		Bids           []*Order
	}

	MatchedOrder struct {
		ID    int64
		Price float64
		Size  float64
	}
)

func StartServer() {
	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler

	// 连接本地 Ethereum 节点（JSON-RPC 接口）
	client, err := ethclient.Dial("http://localhost:8545")
	if err != nil {
		log.Fatal(err)
	}
	ex, err := NewExchange(exchangePrivateKet, client)
	if err != nil {
		log.Fatal(err)
	}

	// pkstr 对应 address = 0x9C939aA98155CBB184854beE77F89dCF1a52F3ac。
	// 这里把它注册成 userID=8，后续请求里的 userID 会用来找到这个用户私钥。
	pkstr8 := "622168571099691cfc787431c14b7deb778bc67f8b9c7a6cf3bcbbb1fb07e414"
	user8 := NewUser(pkstr8, 8)
	ex.Users[user8.ID] = user8

	pkstr7 := "d5aee71596cbca9d39c1085ec79dd5143968880b126bc17452ad90f4f63ecd43"
	user7 := NewUser(pkstr7, 7)
	ex.Users[user7.ID] = user7

	e.GET("/book/:market", ex.handleGetBook)
	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/cancel/:id", ex.cancelOrder)

	// 启动时打印用户地址余额，方便观察下单后链上余额是否发生变化。
	// 注意：这查的是本地以太坊节点里的链上余额，程序重启不会自动回滚已发送交易。
	address7 := "0x36883A1A968074bE4D3c0280A35e640fafc61673"
	balance, _ := ex.Client.BalanceAt(context.Background(), common.HexToAddress(address7), nil)
	fmt.Println("seller7 balance", balance)

	address8 := "0x39AAcf1Dea0fDD769380F7612Af749C6348a68B4"
	balance8, _ := ex.Client.BalanceAt(context.Background(), common.HexToAddress(address8), nil)
	fmt.Println("buyer8 balance", balance8)

	_ = e.Start(":3000")

}

type User struct {
	ID         int64
	PrivateKey *ecdsa.PrivateKey
}

func NewUser(privateKey string, id int64) *User {
	pk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}
	return &User{
		ID:         id,
		PrivateKey: pk,
	}
}

func httpErrorHandler(c *echo.Context, err error) {
	fmt.Println(err)
}

type Exchange struct {
	Client     *ethclient.Client
	Users      map[int64]*User
	orders     map[int64]int64
	privateKey *ecdsa.PrivateKey
	orderbooks map[Market]*orderbook.OrderBook
}

func NewExchange(privateKet string, client *ethclient.Client) (*Exchange, error) {
	orderbooks := make(map[Market]*orderbook.OrderBook)
	orderbooks[MarketETH] = orderbook.NewOrderBook()
	privateKey, err := crypto.HexToECDSA(privateKet)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return &Exchange{
		Client:     client,
		Users:      make(map[int64]*User),
		orders:     make(map[int64]int64),
		privateKey: privateKey,
		orderbooks: orderbooks,
	}, err
}

func (ex *Exchange) handleGetBook(c *echo.Context) error {
	market := Market(c.Param("market"))
	ob, ok := ex.orderbooks[market]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"msg": "market not found",
		})
	}

	orderbookData := OrderbookData{
		TotalAskVolume: ob.AskTotalVolume(),
		TotalBidVolume: ob.BidTotalVolume(),
		Asks:           []*Order{},
		Bids:           []*Order{},
	}
	for _, limit := range ob.Asks() {
		for _, order := range limit.Orders {
			o := Order{
				UserID:    order.UserID,
				ID:        order.ID,
				Price:     limit.Price,
				Size:      order.Size,
				Bid:       order.Bid,
				TimeStamp: order.TimeStamp,
			}
			orderbookData.Asks = append(orderbookData.Asks, &o)
		}
	}

	for _, limit := range ob.Bids() {
		for _, order := range limit.Orders {
			o := Order{
				UserID:    order.UserID,
				ID:        order.ID,
				Price:     limit.Price,
				Size:      order.Size,
				Bid:       order.Bid,
				TimeStamp: order.TimeStamp,
			}
			orderbookData.Bids = append(orderbookData.Bids, &o)
		}
	}

	return c.JSON(http.StatusOK, orderbookData)
}

func (ex *Exchange) handlePlaceMarketOrder(market Market, order *orderbook.Order) ([]orderbook.Match, []*MatchedOrder) {
	ob := ex.orderbooks[market]
	matches := ob.PlaceMarketOrder(order)
	matchOrders := make([]*MatchedOrder, len(matches))

	isBid := false
	if order.Bid {
		isBid = true
	}

	for i := 0; i < len(matches); i++ {
		id := matches[i].Bid.ID
		if isBid {
			id = matches[i].Ask.ID
		}

		matchOrders[i] = &MatchedOrder{
			ID:    id,
			Size:  matches[i].SizeFilled,
			Price: matches[i].Price,
		}
	}

	return matches, matchOrders
}

func (ex *Exchange) handlePlaceLimitOrder(market Market, price float64, order *orderbook.Order) error {
	ob, _ := ex.orderbooks[market]
	ob.PlaceLimitOrder(price, order)

	//// 当前实现把“挂限价单”和“链上转账”绑在了一起：
	//// 下单用户会用自己的私钥签名，把 amount 转到交易所账户。
	//// 这更像是在模拟用户向交易所锁定/托管资产，而不是普通订单簿的内存操作。
	//user := ex.Users[order.UserID]
	//exchangePubKey := ex.privateKey.Public()
	//publicKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)
	//if !ok {
	//	return fmt.Errorf("error casting public key to ECDSA")
	//}
	//toAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	//
	//// amount 的单位是 wei，不是 ETH。
	//// 例如请求 size=10000 时，这里实际转账 10000 wei。
	//// 用户余额减少会等于 amount + gasLimit*gasPrice。
	//amount := big.NewInt(int64(order.Size))
	//
	//return transferETH(ex.Client, user.PrivateKey, toAddress, amount)

	fmt.Printf("placed limit order here => Bd:%t,  price:%0.2f, size:%0.2f\n", order.Bid, order.Limit.Price, order.Size)

	return nil
}

type HandlePlaceOrderResp struct {
	OrderID int64
}

func (ex *Exchange) handlePlaceOrder(c *echo.Context) error {
	var placeOrderData PlaceOrderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&placeOrderData); err != nil {
		return err
	}

	market := placeOrderData.Market
	order := orderbook.NewOrder(placeOrderData.Bid, placeOrderData.Size, placeOrderData.UserId)

	if placeOrderData.Type == LimitOrder {
		if err := ex.handlePlaceLimitOrder(market, placeOrderData.Price, order); err != nil {
			return err
		}
		resp := HandlePlaceOrderResp{
			OrderID: order.ID,
		}
		return c.JSON(http.StatusOK, resp)
	}

	if placeOrderData.Type == MarketOrder {
		matches, matchOrders := ex.handlePlaceMarketOrder(market, order)
		if err := ex.handleMatches(matches); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]any{
			"matches": matchOrders,
		})
	}

	return nil
}

func (ex *Exchange) handleMatches(matches []orderbook.Match) error {
	for _, match := range matches {
		fromUser, ok := ex.Users[match.Ask.UserID]
		if !ok {
			return fmt.Errorf("fromUser not found: %d", match.Ask.UserID)
		}

		toUser, ok := ex.Users[match.Bid.UserID]
		if !ok {
			return fmt.Errorf("askUser not found: %d", match.Bid.UserID)
		}

		//exchangePubKey := ex.privateKey.Public()
		//publicKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)
		//if !ok {
		//	return fmt.Errorf("error casting public key to ECDSA")
		//}
		toAddress := crypto.PubkeyToAddress(toUser.PrivateKey.PublicKey)

		// amount 的单位是 wei，不是 ETH。
		// 例如请求 size=10000 时，这里实际转账 10000 wei。
		// 用户余额减少会等于 amount + gasLimit*gasPrice。
		amount := big.NewInt(int64(match.SizeFilled))

		return utils.TransferETH(ex.Client, fromUser.PrivateKey, toAddress, amount)

	}
	return nil
}

func (ex *Exchange) cancelOrder(c *echo.Context) error {
	idstr := c.Param("id")
	id, _ := strconv.Atoi(idstr)

	ob := ex.orderbooks[MarketETH]
	order := ob.Orders[int64(id)]
	ob.CancelOrder(order)

	return c.JSON(http.StatusOK, map[string]any{
		"msg": "order cancelled",
	})
}
