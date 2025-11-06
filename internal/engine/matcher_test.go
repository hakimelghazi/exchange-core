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
