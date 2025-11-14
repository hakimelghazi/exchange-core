package engine

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/google/uuid"
	dbsqlc "github.com/hakimelghazi/exchange-core/db/sqlc"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Engine struct {
	book    *OrderBook
	matcher *Matcher
	cmds    chan Command
	done    chan struct{}

	pool    *pgxpool.Pool
	queries *dbsqlc.Queries // sqlc-generated queries
}

func NewEngine(buffer int, pool *pgxpool.Pool, queries *dbsqlc.Queries) *Engine {
	book := NewOrderBook()
	return &Engine{
		book:    book,
		matcher: NewMatcher(book),
		cmds:    make(chan Command, buffer),
		done:    make(chan struct{}),
		pool:    pool,
		queries: queries,
	}
}

func (e *Engine) Run(ctx context.Context) {
	defer close(e.done)

	for {
		select {
		case cmd := <-e.cmds:
			switch cmd.Type {

			case CmdPlace:
				e.handlePlace(ctx, cmd)

			case CmdCancel:
				ok := e.book.CancelOrder(cmd.ID)
				cmd.Resp <- cancelResult{OK: ok, Err: nil}
			}

		case <-ctx.Done():
			return
		}
	}
}

func (e *Engine) Place(ctx context.Context, o *Order) (*MatchResult, error) {
	if o == nil {
		return nil, errors.New("nil order")
	}
	resp := make(chan any, 1)
	cmd := Command{Type: CmdPlace, Order: o, Resp: resp}

	if err := e.enqueueCommand(ctx, cmd); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case raw := <-resp:
		out := raw.(placeResult)
		return out.Result, out.Err
	}
}

func (e *Engine) Cancel(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, errors.New("empty order id")
	}
	resp := make(chan any, 1)
	cmd := Command{Type: CmdCancel, ID: id, Resp: resp}

	if err := e.enqueueCommand(ctx, cmd); err != nil {
		return false, err
	}

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case raw := <-resp:
		out := raw.(cancelResult)
		return out.OK, out.Err
	}
}

func (e *Engine) enqueueCommand(ctx context.Context, cmd Command) error {
	select {
	case e.cmds <- cmd:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// insert each trade with sqlc
func (e *Engine) persistTrades(
	ctx context.Context,
	q *dbsqlc.Queries,
	trades []Trade,
) error {
	for _, tr := range trades {
		tradeID, err := newUUID()
		if err != nil {
			return err
		}
		takerID, err := uuidFromString(tr.TakerOrderID)
		if err != nil {
			return err
		}
		makerID, err := uuidFromString(tr.MakerOrderID)
		if err != nil {
			return err
		}

		priceNumeric := numericFromInt64(tr.Price)
		qtyNumeric := numericFromInt64(tr.Quantity)

		// you’ll have whatever args your InsertTrade query expects
		_, err = q.InsertTrade(ctx, dbsqlc.InsertTradeParams{
			ID:           tradeID,
			TakerOrderID: takerID,
			MakerOrderID: makerID,
			Price:        priceNumeric,
			Quantity:     qtyNumeric,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func newUUID() (pgtype.UUID, error) {
	uid, err := uuid.NewRandom()
	if err != nil {
		return pgtype.UUID{}, err
	}
	var out pgtype.UUID
	out.Valid = true
	out.Bytes = uid
	return out, nil
}

func uuidFromString(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return pgtype.UUID{}, err
	}
	var out pgtype.UUID
	out.Valid = true
	out.Bytes = parsed
	return out, nil
}

func numericFromInt64(v int64) pgtype.Numeric {
	return pgtype.Numeric{
		Int:   big.NewInt(v),
		Valid: true,
	}
}

func numericToInt64(v pgtype.Numeric) int64 {
	if !v.Valid || v.Int == nil {
		return 0
	}
	result := new(big.Int).Set(v.Int)
	if v.Exp < 0 {
		divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-v.Exp)), nil)
		result.Quo(result, divisor)
	} else if v.Exp > 0 {
		multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(v.Exp)), nil)
		result.Mul(result, multiplier)
	}
	return result.Int64()
}

func statusFromAmounts(remaining, original int64) string {
	switch {
	case remaining <= 0:
		return "FILLED"
	case remaining < original:
		return "PARTIAL"
	default:
		return "OPEN"
	}
}

func (e *Engine) updateMatchedOrders(
	ctx context.Context,
	q *dbsqlc.Queries,
	taker *Order,
	trades []Trade,
) error {
	filled := make(map[string]int64)
	for _, tr := range trades {
		filled[tr.TakerOrderID] += tr.Quantity
		filled[tr.MakerOrderID] += tr.Quantity
	}

	for orderID := range filled {
		orderUUID, err := uuidFromString(orderID)
		if err != nil {
			return fmt.Errorf("invalid order id %s: %w", orderID, err)
		}

		var originalQty, newRemaining int64
		if orderID == taker.ID {
			originalQty = taker.Quantity
			newRemaining = taker.Remaining
		} else {
			row, err := q.GetOrderForUpdate(ctx, orderUUID)
			if err != nil {
				return err
			}

			currentRemaining := numericToInt64(row.Remaining)
			newRemaining = currentRemaining - filled[orderID]
			if newRemaining < 0 {
				newRemaining = 0
			}
			originalQty = numericToInt64(row.Quantity)
		}

		status := statusFromAmounts(newRemaining, originalQty)
		if err := q.UpdateOrderAfterMatch(ctx, dbsqlc.UpdateOrderAfterMatchParams{
			ID:        orderUUID,
			Remaining: numericFromInt64(newRemaining),
			Status:    status,
		}); err != nil {
			return err
		}
	}

	return nil
}

func orderStatusFromOrder(o *Order) string {
	return statusFromAmounts(o.Remaining, o.Quantity)
}

func (e *Engine) handlePlace(ctx context.Context, cmd Command) {
	res, err := e.matcher.Submit(cmd.Order)
	if err != nil {
		cmd.Resp <- placeResult{Result: res, Err: err}
		return
	}

	if e.pool == nil || e.queries == nil {
		cmd.Resp <- placeResult{Result: res, Err: nil}
		return
	}

	tx, txErr := e.pool.Begin(ctx)
	if txErr != nil {
		cmd.Resp <- placeResult{Result: res, Err: txErr}
		return
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := e.queries.WithTx(tx)

	orderUUID, err := uuidFromString(cmd.Order.ID)
	if err != nil {
		cmd.Resp <- placeResult{Result: res, Err: fmt.Errorf("invalid order id: %w", err)}
		return
	}
	userUUID, err := uuidFromString(cmd.Order.UserID)
	if err != nil {
		cmd.Resp <- placeResult{Result: res, Err: fmt.Errorf("invalid user id: %w", err)}
		return
	}

	_, err = qtx.UpsertOrder(ctx, dbsqlc.UpsertOrderParams{
		ID:        orderUUID,
		UserID:    userUUID,
		Market:    cmd.Order.Market,
		Side:      string(cmd.Order.Side),
		Price:     numericFromInt64(cmd.Order.Price),
		Quantity:  numericFromInt64(cmd.Order.Quantity),
		Remaining: numericFromInt64(cmd.Order.Remaining),
		Status:    orderStatusFromOrder(cmd.Order),
	})
	if err != nil {
		cmd.Resp <- placeResult{Result: res, Err: err}
		return
	}

	if len(res.Trades) > 0 {
		if err := e.persistTrades(ctx, qtx, res.Trades); err != nil {
			cmd.Resp <- placeResult{Result: res, Err: err}
			return
		}
		if err := e.updateMatchedOrders(ctx, qtx, cmd.Order, res.Trades); err != nil {
			cmd.Resp <- placeResult{Result: res, Err: err}
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		cmd.Resp <- placeResult{Result: res, Err: err}
		return
	}
	tx = nil

	cmd.Resp <- placeResult{Result: res, Err: nil}
}
