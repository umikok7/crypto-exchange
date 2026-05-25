package orderbook

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

type Match struct {
	Ask        *Order
	Bid        *Order
	SizeFilled float64
	Price      float64
}

// Order 代表订单簿中的一笔订单。
// Size: 买卖数量
// Bid: true = 买单, false = 卖单
// Limit: 指向所属的价格档位
// TimeStamp: 纳秒时间戳，用于排序
type Order struct {
	ID        int64
	Size      float64
	Bid       bool
	Limit     *Limit
	TimeStamp int64
}

type Orders []*Order

func (o Orders) Len() int           { return len(o) }
func (o Orders) Swap(i, j int)      { o[i], o[j] = o[j], o[i] }
func (o Orders) Less(i, j int) bool { return o[i].TimeStamp < o[j].TimeStamp }

func NewOrder(bid bool, size float64) *Order {
	return &Order{
		ID:        int64(rand.Intn(1000000)),
		Size:      size,
		Bid:       bid,
		TimeStamp: time.Now().UnixNano(),
	}
}

func (o *Order) String() string {
	return fmt.Sprintf("[size: %.2f]", o.Size)
}

func (o *Order) isFilled() bool {
	return o.Size == 0.0
}

// Limit 代表订单簿中的一个价格档位。
// Price: 价格
// Orders: 该价格下的所有订单切片
// TotalVolume: 该价格档位的总数量
type Limit struct {
	Price       float64
	Orders      Orders
	TotalVolume float64
}

type Limits []*Limit

type ByBestAsk struct {
	Limits
}

func (a ByBestAsk) Len() int           { return len(a.Limits) }
func (a ByBestAsk) Swap(i, j int)      { a.Limits[i], a.Limits[j] = a.Limits[j], a.Limits[i] }
func (a ByBestAsk) Less(i, j int) bool { return a.Limits[i].Price < a.Limits[j].Price }

type ByBestBid struct {
	Limits
}

func (b ByBestBid) Len() int           { return len(b.Limits) }
func (b ByBestBid) Swap(i, j int)      { b.Limits[i], b.Limits[j] = b.Limits[j], b.Limits[i] }
func (b ByBestBid) Less(i, j int) bool { return b.Limits[i].Price > b.Limits[j].Price }

func NewLimit(price float64) *Limit {
	return &Limit{
		Price:  price,
		Orders: []*Order{},
	}
}

func (l *Limit) AddOrder(o *Order) {
	o.Limit = l
	l.Orders = append(l.Orders, o)
	l.TotalVolume += o.Size
}

func (l *Limit) DeleteOrder(o *Order) {
	for i := 0; i < len(l.Orders); i++ {
		if l.Orders[i] == o {
			l.Orders[i] = l.Orders[len(l.Orders)-1]
			l.Orders = l.Orders[:len(l.Orders)-1]
			break
		}
	}
	o.Limit = nil
	l.TotalVolume -= o.Size

	sort.Sort(l.Orders)

	// TODO: resort the whole resting order

}

// Fill 将传入的订单 o 与当前价格档位中的所有订单进行撮合，
// 逐个执行 fillOrder 直到 o 被完全成交或档位订单耗尽。
// 返回所有撮合结果（Match），并从当前档位中移除已成交完毕的订单。
func (l *Limit) Fill(o *Order) []Match {
	var (
		matchs        []Match
		orderToDelete []*Order
	)

	for _, order := range l.Orders {
		match := l.fillOrder(order, o)
		matchs = append(matchs, match)
		l.TotalVolume -= match.SizeFilled

		if order.isFilled() {
			orderToDelete = append(orderToDelete, order)
		}

		if o.isFilled() {
			break
		}
	}

	for _, order := range orderToDelete {
		l.DeleteOrder(order)
	}

	return matchs
}

func (l *Limit) fillOrder(a, b *Order) Match {
	var (
		bid        *Order
		ask        *Order
		sizeFilled float64
	)

	if a.Bid {
		bid = a
		ask = b
	} else {
		bid = b
		ask = a
	}

	if a.Size > b.Size {
		a.Size -= b.Size
		sizeFilled = b.Size
		b.Size = 0.0
	} else {
		b.Size -= a.Size
		sizeFilled = a.Size
		a.Size = 0.0
	}
	return Match{
		Ask:        ask,
		Bid:        bid,
		SizeFilled: sizeFilled,
		Price:      l.Price,
	}
}

// OrderBook 是订单簿，包含所有卖单(asks)和买单(bids)
// AskLimits/BidLimits: 价格到价格档位的映射，用于快速查找
type OrderBook struct {
	asks []*Limit
	bids []*Limit

	AskLimits map[float64]*Limit
	BidLimits map[float64]*Limit

	Orders map[int64]*Order
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		asks:      []*Limit{},
		bids:      []*Limit{},
		AskLimits: make(map[float64]*Limit),
		BidLimits: make(map[float64]*Limit),
		Orders:    make(map[int64]*Order),
	}
}

// PlaceMarketOrder 执行市价单撮合。
// 市价单会立即与对手方订单簿中最佳价格档位进行撮合，直到指定数量全部成交。
// 买单与卖单(asks)撮合，卖单与买单(bids)撮合。
// 若对手方流动性不足则 panic。
// 返回所有撮合结果。
func (ob *OrderBook) PlaceMarketOrder(o *Order) []Match {
	matchs := []Match{}

	if o.Bid {
		if o.Size > ob.AskTotalVolume() {
			panic(fmt.Errorf("not enough volume [size: %.2f] for market order [size: %.2f]", ob.AskTotalVolume(), o.Size))
		}
		for _, limit := range ob.Asks() {
			limitMatches := limit.Fill(o)
			matchs = append(matchs, limitMatches...)

			if len(limit.Orders) == 0 {
				ob.DeleteLimit(false, limit)
			}

			// 市价单已完全成交后必须停止继续扫描后续价位，
			// 否则会对剩余档位生成 SizeFilled 为 0 的无效 Match。
			if o.isFilled() {
				break
			}
		}
	} else {
		if o.Size > ob.BidTotalVolume() {
			panic(fmt.Errorf("not enough volume [size: %.2f] for market order [size: %.2f]", ob.BidTotalVolume(), o.Size))
		}
		for _, limit := range ob.Bids() {
			limitMatches := limit.Fill(o)
			matchs = append(matchs, limitMatches...)

			if len(limit.Orders) == 0 {
				ob.DeleteLimit(true, limit)
			}

			if o.isFilled() {
				break
			}
		}
	}

	return matchs
}

// PlaceLimitOrder 添加一笔限价单到订单簿。
// price: 指定的成交价格
// o: 订单，包含买卖方向和数量
// 若该价格档位已存在，则将订单追加到该档位；否则创建新档位。
// 限价单不会立即撮合，而是挂在订单簿中等待对手方。
func (ob *OrderBook) PlaceLimitOrder(price float64, o *Order) {
	var limit *Limit
	if o.Bid {
		limit = ob.BidLimits[price]
	} else {
		limit = ob.AskLimits[price]
	}

	if limit == nil {
		limit = NewLimit(price)

		if o.Bid {
			ob.bids = append(ob.bids, limit)
			ob.BidLimits[price] = limit
		} else {
			ob.asks = append(ob.asks, limit)
			ob.AskLimits[price] = limit
		}
	}
	ob.Orders[o.ID] = o
	limit.AddOrder(o)
}

func (ob *OrderBook) DeleteLimit(bid bool, l *Limit) {
	if bid {
		delete(ob.BidLimits, l.Price)

		for i := 0; i < len(ob.bids); i++ {
			if ob.bids[i] == l {
				ob.bids[i] = ob.bids[len(ob.bids)-1]
				ob.bids = ob.bids[:len(ob.bids)-1]
			}
		}
	} else {
		delete(ob.AskLimits, l.Price)

		for i := 0; i < len(ob.asks); i++ {
			if ob.asks[i] == l {
				ob.asks[i] = ob.asks[len(ob.asks)-1]
				ob.asks = ob.asks[:len(ob.asks)-1]
			}
		}
	}
}

func (ob *OrderBook) CancelOrder(o *Order) {
	limit := o.Limit
	limit.DeleteOrder(o)
	delete(ob.Orders, o.ID)
}

func (ob *OrderBook) BidTotalVolume() float64 {
	totalVolume := 0.0
	for i := 0; i < len(ob.bids); i++ {
		totalVolume += ob.bids[i].TotalVolume
	}
	return totalVolume
}

func (ob *OrderBook) AskTotalVolume() float64 {
	totalVolume := 0.0
	for i := 0; i < len(ob.asks); i++ {
		totalVolume += ob.asks[i].TotalVolume
	}
	return totalVolume
}

// Asks 返回按价格从低到高排序的卖单列表
func (ob *OrderBook) Asks() []*Limit {
	sort.Sort(ByBestAsk{ob.asks})
	return ob.asks
}

// Bids 返回按价格从高到低排序的买单列表
func (ob *OrderBook) Bids() []*Limit {
	sort.Sort(ByBestBid{ob.bids})
	return ob.bids
}
