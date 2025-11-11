package engine

import (
	"context"

	dbsqlc "github.com/hakimelghazi/exchange-core/db/sqlc"
)

type Engine struct {
	book    *OrderBook
	matcher *Matcher
	cmds    chan Command
	done    chan struct{}
	queries *dbsqlc.Queries
}

// NewEngine creates the loop. buffer lets you tune backpressure.
func NewEngine(buffer int, queries *dbsqlc.Queries) *Engine {
	book := NewOrderBook()
	return &Engine{
		book:    book,
		matcher: NewMatcher(book),
		cmds:    make(chan Command, buffer),
		done:    make(chan struct{}),
		queries: queries, // store it
	}
}

// Run processes commands until ctx is canceled or Stop() is called.
func (e *Engine) Run(ctx context.Context) {
	defer close(e.done)
	for {
		select {
		case cmd := <-e.cmds:
			switch cmd.Type {
			case CmdPlace:
				res, err := e.matcher.Submit(cmd.Order)
				cmd.Resp <- struct {
					Result *MatchResult
					Err    error
				}{res, err}
			case CmdCancel:
				ok := e.book.CancelOrder(cmd.ID)
				cmd.Resp <- struct {
					OK  bool
					Err error
				}{ok, nil}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) Stop() { close(e.cmds); <-e.done }

// ----- Client helpers (type-safe wrappers) -----

func (e *Engine) Place(ctx context.Context, o *Order) (*MatchResult, error) {
	resp := make(chan any, 1)
	select {
	case e.cmds <- Command{Type: CmdPlace, Order: o, Resp: resp}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case v := <-resp:
		r := v.(struct {
			Result *MatchResult
			Err    error
		})
		return r.Result, r.Err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *Engine) Cancel(ctx context.Context, id string) (bool, error) {
	resp := make(chan any, 1)
	select {
	case e.cmds <- Command{Type: CmdCancel, ID: id, Resp: resp}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	select {
	case v := <-resp:
		r := v.(struct {
			OK  bool
			Err error
		})
		return r.OK, r.Err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}
