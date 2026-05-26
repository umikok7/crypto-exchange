package main

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
)

const (
	// exchangePrivateKet 是交易所账户的私钥。
	// 当前原型里，用户下限价单时会把 ETH 从用户账户转到这个交易所账户。
	exchangePrivateKet = "5f5229f4e3dbaeeee4c6407393314b91c53d106aa4cb9a959767f34d1e696c6b"

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

func main() {
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
	pkstr := "4c7b0d4f5cca7e4449b2733463969b594cdb04f685f972ec2bc186b4e8958bea"
	pk, err := crypto.HexToECDSA(pkstr)
	if err != nil {
		log.Fatal(err)
	}
	user := &User{
		ID:         8,
		PrivateKey: pk,
	}
	ex.Users[user.ID] = user

	e.GET("/book/:market", ex.handleGetBook)
	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/cancel/:id", ex.cancelOrder)

	// 启动时打印用户地址余额，方便观察下单后链上余额是否发生变化。
	// 注意：这查的是本地以太坊节点里的链上余额，程序重启不会自动回滚已发送交易。
	address := "0x9C939aA98155CBB184854beE77F89dCF1a52F3ac"
	balance, _ := ex.Client.BalanceAt(context.Background(), common.HexToAddress(address), nil)
	fmt.Println("balance", balance)

	//// 将十六进制私钥字符串转换为 ECDSA 私钥对象，用于签名交易
	//privateKey, err := crypto.HexToECDSA(exchangePrivateKet)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//// 从私钥推导出公钥，再得到以太坊地址
	//publicKey := privateKey.Public()
	//publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	//if !ok {
	//	log.Fatal("error casting public key to ECDSA")
	//}
	//
	//fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	//
	//// 查询发送账户当前的待处理交易 nonce，防止重放攻击
	//nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//// 设置交易参数：转账金额为 1 ETH (10^18 wei)
	//value := big.NewInt(1000000000000000000)
	//
	//// ETH 转账的标准 gas 上限为 21000
	//gasLimit := uint64(21000)
	//
	//// 从节点获取当前市场建议的 gas 价格
	//gasPrice, err := client.SuggestGasPrice(context.Background())
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//// 目标地址
	//ToAddress := common.HexToAddress("0xAEA6718FEA800465918B275f652B5037312cDb12")
	//
	//// 创建交易对象：nonce, to, value, gasLimit, gasPrice, data(nil)
	//tx := types.NewTransaction(nonce, ToAddress, value, gasLimit, gasPrice, nil)
	//
	//// 链 ID：1337 是本地开发链（Ganache）的默认 ID
	//chainID := big.NewInt(1337)
	//
	//// 使用 EIP155 签名规范对交易进行签名（包含 chainID 防止跨链重放攻击）
	//signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//// 将签名后的交易提交到以太坊节点，节点会广播到网络
	//if err = client.SendTransaction(context.Background(), signedTx); err != nil {
	//	log.Fatal(err)
	//}
	//
	//// 查询目标地址的 ETH 余额
	//balance, err := client.BalanceAt(context.Background(), ToAddress, nil)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//fmt.Println("balance", balance)

	_ = e.Start(":3000")

}

type User struct {
	ID         int64
	PrivateKey *ecdsa.PrivateKey
}

func NewUser(privateKey string) *User {
	pk, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}
	return &User{
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

	// 当前实现把“挂限价单”和“链上转账”绑在了一起：
	// 下单用户会用自己的私钥签名，把 amount 转到交易所账户。
	// 这更像是在模拟用户向交易所锁定/托管资产，而不是普通订单簿的内存操作。
	user := ex.Users[order.UserID]
	exchangePubKey := ex.privateKey.Public()
	publicKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("error casting public key to ECDSA")
	}
	toAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	// amount 的单位是 wei，不是 ETH。
	// 例如请求 size=10000 时，这里实际转账 10000 wei。
	// 用户余额减少会等于 amount + gasLimit*gasPrice。
	amount := big.NewInt(int64(order.Size))

	return transferETH(ex.Client, user.PrivateKey, toAddress, amount)
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
		return c.JSON(http.StatusOK, map[string]any{
			"msg": "order placed",
		})
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
