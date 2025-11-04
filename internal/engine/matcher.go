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

	// 1. while we have quantity to fill AND best ask is affordable → match
	// 2. otherwise, if it's a limit order, rest it on the bid side
	// (implementation to fill in)
	return res, nil
}

func (m *Matcher) matchSell(o *Order) (*MatchResult, error) {
	// symmetric to matchBuy
	res := &MatchResult{Trades: make([]Trade, 0)}
	return res, nil
}
