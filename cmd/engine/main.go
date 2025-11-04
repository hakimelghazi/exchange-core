package main

import (
	"fmt"
	"time"

	"github.com/yourname/exchange-core/internal/engine"
)

func main() {
	book := engine.NewOrderBook()
	m := engine.NewMatcher(book)

	// Maker: someone wants to SELL 1 @ 100
	sell := &engine.Order{
		ID:        "sell-1",
		UserID:    "user-b",
		Market:    engine.MarketBTCUSD,
		Side:      engine.SideSell,
		Price:     100,
		Quantity:  1_00000000, // 1 BTC in sats, adjust to your unit
		Remaining: 1_00000000,
		CreatedAt: time.Now(),
	}
	// later: m.Submit(sell)

	// Taker: someone wants to BUY 1 @ 100
	buy := &engine.Order{
		ID:        "buy-1",
		UserID:    "user-a",
		Market:    engine.MarketBTCUSD,
		Side:      engine.SideBuy,
		Price:     100,
		Quantity:  1_00000000,
		Remaining: 1_00000000,
		CreatedAt: time.Now(),
	}

	// submit maker first so it rests
	_, _ = m.Submit(sell)
	res, _ := m.Submit(buy)

	fmt.Printf("trades: %+v\n", res.Trades)
}
