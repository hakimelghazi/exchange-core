package engine

import (
	"errors"
	"strings"
	"time"
)

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

var ErrInvalidSide = errors.New("invalid order side")

func ParseSide(s string) (Side, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case string(SideBuy):
		return SideBuy, nil
	case string(SideSell):
		return SideSell, nil
	default:
		return "", ErrInvalidSide
	}
}
