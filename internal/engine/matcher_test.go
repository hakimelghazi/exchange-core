package engine

import "testing"

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
