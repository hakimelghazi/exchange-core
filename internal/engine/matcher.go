package engine

type Trade struct {
	TakerOrderID string
	MakerOrderID string
	Price        int64
	Quantity     int64
}

type MatchResult struct {
	Trades      []Trade
	OrderFilled bool   // true if incoming is fully filled
	Remainder   *Order // if partially filled, the remaining resting order
}

type Matcher struct {
	book *OrderBook
}

func NewMatcher(book *OrderBook) *Matcher {
	return &Matcher{book: book}
}

// Submit takes an incoming order and matches it against the opposite side.
// Later we can add: order types, time-in-force, self-trade prevention.
func (m *Matcher) Submit(o *Order) (*MatchResult, error) {
	if o.Side == SideBuy {
		return m.matchBuy(o)
	}
	return m.matchSell(o)
}

func (m *Matcher) matchBuy(o *Order) (*MatchResult, error) {
	res := &MatchResult{Trades: make([]Trade, 0)}
	remaining := o.Remaining

	for remaining > 0 {
		bestAsk := m.book.bestAsk()
		if bestAsk == nil {
			break
		}
		// if limit order and best ask is too expensive, stop
		if !o.IsMarket && bestAsk.price > o.Price {
			break
		}
		// oldest maker at this price
		front := bestAsk.orders.Front()
		maker := front.Value.(*Order)

		// how much can be traded
		qty := min(remaining, maker.Remaining)

		// emit trade at maker price
		res.Trades = append(res.Trades, Trade{
			TakerOrderID: o.ID,
			MakerOrderID: maker.ID,
			Price:        bestAsk.price,
			Quantity:     qty,
		})

		// decrement
		remaining -= qty
		maker.Remaining -= qty

		// if maker is filled, pop it
		if maker.Remaining == 0 {
			bestAsk.orders.Remove(front)
		}
		// if price level emptuy, remove it
		if bestAsk.orders.Len() == 0 {
			m.book.removeAskLevel(bestAsk.price)
		}
	}

	if remaining == 0 {
		res.OrderFilled = true
		return res, nil
	}

	o.Remaining = remaining
	if !o.IsMarket {
		//rest remainder on bid side
		m.book.AddOrder(o)
		res.Remainder = o
	} else {
		// market order, return unfilled part
		res.Remainder = o
	}
	return res, nil
}

func (m *Matcher) matchSell(o *Order) (*MatchResult, error) {
	// symmetric to matchBuy
	res := &MatchResult{Trades: make([]Trade, 0)}
	remaining := o.Remaining

	for remaining > 0 {
		bestBid := m.book.bestBid()
		if bestBid == nil {
			break
		}

		if !o.IsMarket && bestBid.price < o.Price {
			break
		}

		front := bestBid.orders.Front()
		maker := front.Value.(*Order)

		qty := min(remaining, maker.Remaining)

		res.Trades = append(res.Trades, Trade{
			TakerOrderID: o.ID,
			MakerOrderID: maker.ID,
			Price:        bestBid.price,
			Quantity:     qty,
		})

		remaining -= qty
		maker.Remaining -= qty

		if maker.Remaining == 0 {
			bestBid.orders.Remove(front)
		}

		if bestBid.orders.Len() == 0 {
			m.book.removeBidLevel(bestBid.price)
		}
	}

	if remaining == 0 {
		res.OrderFilled = true
		return res, nil
	}

	o.Remaining = remaining
	if !o.IsMarket {
		m.book.AddOrder(o)
		res.Remainder = o
	} else {
		res.Remainder = o
	}
	return res, nil
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
