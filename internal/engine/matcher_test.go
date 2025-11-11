package engine

import (
	"strconv"
	"testing"
)

func TestFullFill(t *testing.T) {
	ob := NewOrderBook()
	o1 := newTestOrder("o1", SideSell, 100, 1)

	ob.AddOrder(o1)
	o2 := newTestOrder("o2", SideBuy, 100, 1)

	m := NewMatcher(ob)

	m.Submit(o2)

	_, ok1 := ob.ordersByID["o1"]
	_, ok2 := ob.ordersByID["o2"]

	if ok1 || ok2 {
		t.Fatalf("either order o1 or 2 was found in orderbook")
	}

	if len(ob.askPrices) != 0 || len(ob.bidPrices) != 0 {
		t.Fatalf("expected empty book")
	}

	if len(ob.ordersByID) != 0 {
		t.Fatalf("expected ordersByID to be empty")
	}

}

func TestPartialFill(t *testing.T) {
	ob := NewOrderBook()
	m := NewMatcher(ob)
	o1 := newTestOrder("o1", SideBuy, 105, 2)
	ob.AddOrder(o1)

	o2 := newTestOrder("o2", SideSell, 104, 1)

	m.Submit(o2)

	ref1, ok1 := ob.ordersByID["o1"]

	if !ok1 {
		t.Fatalf("order 1 was removed")
	}

	// check here
	if ref1.side != SideBuy || ref1.price != 105 || ref1.elem.Value.(*Order).Remaining != 1 {
		t.Fatalf("order 1 was modified")
	}

	_, ok2 := ob.ordersByID["o2"]

	if ok2 {
		t.Fatalf("order 2 was not properly removed after being filled")
	}

}

func TestNoMatch(t *testing.T) {

	ob := NewOrderBook()
	m := NewMatcher(ob)

	o1 := newTestOrder("o1", SideSell, 130, 3)
	m.Submit(o1)

	o2 := newTestOrder("o2", SideBuy, 110, 1)

	m.Submit(o2)

	_, ok1 := ob.ordersByID["o1"]
	_, ok2 := ob.ordersByID["o2"]

	if !ok1 || !ok2 {
		t.Fatalf("order was removed")
	}

	if len(ob.askPrices) != 1 || len(ob.bidPrices) != 1 {
		t.Fatalf("expected 1 ask and 1 bid")
	}

}

func TestMarketWalk(t *testing.T) {
	ob := NewOrderBook()
	m := NewMatcher(ob)

	for i := range 10 {
		price := int64(100 + i)
		order_id := "o" + strconv.Itoa(i)
		new_order := newTestOrder(order_id, SideSell, price, 1)

		m.Submit(new_order)
	}

	o_test := newTestOrder("o_test", SideBuy, 115, 5)

	m.Submit(o_test)

	for i := range 5 {
		order_id := "o" + strconv.Itoa(i)

		_, ok := ob.ordersByID[order_id]

		if ok {
			t.Fatalf("orders not filled")
		}

	}

	for i := 5; i < 10; i++ {
		order_id := "o" + strconv.Itoa(i)

		_, ok := ob.ordersByID[order_id]

		if !ok {
			t.Fatalf("orders missing")
		}

	}

	if _, ok := ob.ordersByID["o_test"]; ok {
		t.Fatalf("o_test should be fully filled and not resting")
	}

	if len(ob.askPrices) != 5 {
		t.Fatalf("expected 5 ask price levels left, got %d", len(ob.askPrices))
	}

}
