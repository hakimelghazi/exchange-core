package engine

import "time"

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

const MarketBTCUSD = "BTC-USD"

type Order struct {
	ID        string
	UserID    string
	Market    string
	Side      Side
	Price     int64 // integer price (ticks)
	Quantity  int64 // original quantity
	Remaining int64 // unfilled
	IsMarket  bool
	CreatedAt time.Time
}
