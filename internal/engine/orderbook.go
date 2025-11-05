package engine

import (
	"container/list"
	"sort"
)

// priceLevel holds FIFO orders for one price.
type priceLevel struct {
	price  int64
	orders *list.List // of *Order, oldest first
}

type OrderBook struct {
	// key = price, value = *priceLevel
	bids map[int64]*priceLevel
	asks map[int64]*priceLevel

	// to keep prices sorted; for a single market and MVP this is fine
	bidPrices []int64 // sorted desc
	askPrices []int64 // sorted asc

	ordersByID map[string]*orderRef
}

type orderRef struct {
	side  Side
	price int64
	elem  *list.Element // points into level.orders list
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:       make(map[int64]*priceLevel),
		asks:       make(map[int64]*priceLevel),
		bidPrices:  make([]int64, 0),
		askPrices:  make([]int64, 0),
		ordersByID: make(map[string]*orderRef),
	}
}

func (ob *OrderBook) AddOrder(o *Order) {
	if o.Side == SideBuy {
		lvl, ok := ob.bids[o.Price]

		if !ok {
			lvl = &priceLevel{price: o.Price, orders: list.New()}
			ob.bids[o.Price] = lvl
			ob.insertBidPrice(o.Price)
		}
		elem := lvl.orders.PushBack(o)

		ob.ordersByID[o.ID] = &orderRef{
			side:  SideBuy,
			price: o.Price,
			elem:  elem,
		}
		return
	}

	// sell side
	lvl, ok := ob.asks[o.Price]
	if !ok {
		lvl = &priceLevel{price: o.Price, orders: list.New()}
		ob.asks[o.Price] = lvl
		ob.insertAskPrice(o.Price)
	}
	elem := lvl.orders.PushBack(o)

	ob.ordersByID[o.ID] = &orderRef{
		side:  SideSell,
		price: o.Price,
		elem:  elem,
	}
}

// cancel order using OrdersByID
func (ob *OrderBook) CancelOrder(id string) bool {
	ref, ok := ob.ordersByID[id]
	if !ok {
		return false
	}
	var lvl *priceLevel
	if ref.side == SideBuy {
		lvl = ob.bids[ref.price]
	} else {
		lvl = ob.asks[ref.price]
	}
	lvl.orders.Remove(ref.elem)
	if lvl.orders.Len() == 0 {
		if ref.side == SideBuy {
			ob.removeBidLevel(ref.price)
		} else {
			ob.removeAskLevel(ref.price)
		}
	}
	delete(ob.ordersByID, id)
	return true
}

// bids sorted in descending order
func (ob *OrderBook) insertBidPrice(price int64) {
	ob.bidPrices = append(ob.bidPrices, price)
	sort.Slice(ob.bidPrices, func(i, j int) bool {
		return ob.bidPrices[i] > ob.bidPrices[j]
	})
}

// asks sorted in ascending order
func (ob *OrderBook) insertAskPrice(price int64) {
	ob.askPrices = append(ob.askPrices, price)
	sort.Slice(ob.askPrices, func(i, j int) bool {
		return ob.askPrices[i] < ob.askPrices[j]
	})
}

func (ob *OrderBook) bestBid() *priceLevel {
	if len(ob.bidPrices) == 0 {
		return nil
	}
	p := ob.bidPrices[0]
	return ob.bids[p]
}

func (ob *OrderBook) bestAsk() *priceLevel {
	if len(ob.askPrices) == 0 {
		return nil
	}

	p := ob.askPrices[0]
	return ob.asks[p]
}

func (ob *OrderBook) removeBidLevel(price int64) {
	delete(ob.bids, price)
	for i, p := range ob.bidPrices {
		if p == price {
			ob.bidPrices = append(ob.bidPrices[:i], ob.bidPrices[i+1:]...)
			break
		}
	}
}

func (ob *OrderBook) removeAskLevel(price int64) {
	delete(ob.asks, price)
	for i, p := range ob.askPrices {
		if p == price {
			ob.askPrices = append(ob.askPrices[:i], ob.askPrices[i+1:]...)
			break
		}
	}
}
