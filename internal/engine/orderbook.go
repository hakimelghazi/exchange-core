package engine

import (
	"container/list"
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
}

func NewOrderBook() *OrderBook {
	return &OrderBook{
		bids:      make(map[int64]*priceLevel),
		asks:      make(map[int64]*priceLevel),
		bidPrices: make([]int64, 0),
		askPrices: make([]int64, 0),
	}
}
