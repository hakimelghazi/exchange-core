package engine

import (
	"testing"
	"time"
)

func newTestOrder(id string, side Side, price, qty int64) *Order {
	return &Order{
		ID:        id,
		UserID:    "u1",
		Market:    MarketBTCUSD,
		Side:      side,
		Price:     price,
		Quantity:  qty,
		Remaining: qty,
		CreatedAt: time.Now(),
	}
}

func TestAddOrderStoresInLookup(t *testing.T) {
	ob := NewOrderBook()
	o := newTestOrder("o1", SideBuy, 100, 10)
	ob.AddOrder(o)

	ref, ok := ob.ordersByID["o1"]
	if !ok {
		t.Fatalf("order not found in ordersByID")
	}
	if ref.side != SideBuy || ref.price != 100 {
		t.Fatalf("unexpected ref: %+v", ref)
	}
}

func TestCancelOrderRemovesFromLevel(t *testing.T) {
	ob := NewOrderBook()
	o1 := newTestOrder("o1", SideSell, 105, 5)
	o2 := newTestOrder("o2", SideSell, 105, 5)
	ob.AddOrder(o1)
	ob.AddOrder(o2)

	ok := ob.CancelOrder("o1")
	if !ok {
		t.Fatalf("expected cancel to succeed")
	}

	// level should still exist (o2 still there)
	lvl := ob.asks[105]
	if lvl == nil || lvl.orders.Len() != 1 {
		t.Fatalf("expected one order left at level 105")
	}
	if _, still := ob.ordersByID["o1"]; still {
		t.Fatalf("expected o1 to be removed from lookup")
	}
}

func TestCancelLastOrderRemovesLevel(t *testing.T) {
	ob := NewOrderBook()
	o1 := newTestOrder("o1", SideBuy, 99, 5)
	ob.AddOrder(o1)

	ok := ob.CancelOrder("o1")
	if !ok {
		t.Fatalf("expected cancel to succeed")
	}

	if len(ob.bidPrices) != 0 {
		t.Fatalf("expected bidPrices to be empty, got %v", ob.bidPrices)
	}
	if _, ok := ob.bids[99]; ok {
		t.Fatalf("expected bids[99] to be removed")
	}
}
