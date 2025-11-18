package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/google/uuid"
	dbsqlc "github.com/hakimelghazi/exchange-core/db/sqlc"
	"github.com/jackc/pgx/v5"
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

func NewEngine(buffer int, pool *pgxpool.Pool, queries *dbsqlc.Queries) (*Engine, error) {
	if pool == nil || queries == nil {
		return nil, errors.New("engine requires a persistent database connection")
	}
	book := NewOrderBook()
	return &Engine{
		book:    book,
		matcher: NewMatcher(book),
		cmds:    make(chan Command, buffer),
		done:    make(chan struct{}),
		pool:    pool,
		queries: queries,
	}, nil
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
				ok, err := e.handleCancel(ctx, cmd.ID)
				cmd.Resp <- cancelResult{OK: ok, Err: err}
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

// Persist both trades and their ledger postings in a single transaction.
func (e *Engine) persistTradesAndLedger(
	ctx context.Context,
	q *dbsqlc.Queries,
	trades []Trade,
) error {
	for _, tr := range trades {
		tradeID := mustNewUUID()
		takerID, err := uuidFromString(tr.TakerOrderID)
		if err != nil {
			return err
		}
		makerID, err := uuidFromString(tr.MakerOrderID)
		if err != nil {
			return err
		}

		if _, err := q.InsertTrade(ctx, dbsqlc.InsertTradeParams{
			ID:           tradeID,
			TakerOrderID: takerID,
			MakerOrderID: makerID,
			Price:        numericFromInt64(tr.Price),
			Quantity:     numericFromInt64(tr.Quantity),
		}); err != nil {
			return err
		}

		ledgerID := mustNewUUID()
		if _, err := q.CreateLedger(ctx, dbsqlc.CreateLedgerParams{
			ID:      ledgerID,
			RefType: "trade",
			RefID:   tradeID,
		}); err != nil {
			return err
		}

		takerRow, err := q.GetOrderForUpdate(ctx, takerID)
		if err != nil {
			return err
		}
		makerRow, err := q.GetOrderForUpdate(ctx, makerID)
		if err != nil {
			return err
		}

		notional := new(big.Int).Mul(big.NewInt(tr.Price), big.NewInt(tr.Quantity))
		amtUSD := pgtype.Numeric{Int: notional, Valid: true}
		amtBTC := numericFromInt64(tr.Quantity)

		var buyerUser, sellerUser uuid.UUID
		if takerRow.Side == "BUY" {
			buyerUser = uuid.UUID(takerRow.UserID.Bytes)
			sellerUser = uuid.UUID(makerRow.UserID.Bytes)
		} else {
			buyerUser = uuid.UUID(makerRow.UserID.Bytes)
			sellerUser = uuid.UUID(takerRow.UserID.Bytes)
		}

		buyerUSD, err := e.getOrCreateAccountID(ctx, q, buyerUser, "USD")
		if err != nil {
			return err
		}
		buyerBTC, err := e.getOrCreateAccountID(ctx, q, buyerUser, "BTC")
		if err != nil {
			return err
		}
		sellerUSD, err := e.getOrCreateAccountID(ctx, q, sellerUser, "USD")
		if err != nil {
			return err
		}
		sellerBTC, err := e.getOrCreateAccountID(ctx, q, sellerUser, "BTC")
		if err != nil {
			return err
		}

		if err := q.InsertLedgerEntry(ctx, dbsqlc.InsertLedgerEntryParams{
			ID:        mustNewUUID(),
			LedgerID:  ledgerID,
			AccountID: buyerUSD,
			Amount:    negate(amtUSD),
		}); err != nil {
			return err
		}
		if err := q.InsertLedgerEntry(ctx, dbsqlc.InsertLedgerEntryParams{
			ID:        mustNewUUID(),
			LedgerID:  ledgerID,
			AccountID: buyerBTC,
			Amount:    amtBTC,
		}); err != nil {
			return err
		}
		if err := q.InsertLedgerEntry(ctx, dbsqlc.InsertLedgerEntryParams{
			ID:        mustNewUUID(),
			LedgerID:  ledgerID,
			AccountID: sellerBTC,
			Amount:    negate(amtBTC),
		}); err != nil {
			return err
		}
		if err := q.InsertLedgerEntry(ctx, dbsqlc.InsertLedgerEntryParams{
			ID:        mustNewUUID(),
			LedgerID:  ledgerID,
			AccountID: sellerUSD,
			Amount:    amtUSD,
		}); err != nil {
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

func mustNewUUID() pgtype.UUID {
	uid, _ := newUUID()
	return uid
}

func pgUUIDFrom(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{
		Bytes: u,
		Valid: true,
	}
}

func (e *Engine) getOrCreateAccountID(
	ctx context.Context,
	q *dbsqlc.Queries,
	user uuid.UUID,
	asset string,
) (pgtype.UUID, error) {
	uid := pgUUIDFrom(user)
	acc, err := q.GetAccountByUserAsset(ctx, dbsqlc.GetAccountByUserAssetParams{
		UserID: uid,
		Asset:  asset,
	})
	if err == nil && acc.ID.Valid {
		return acc.ID, nil
	}

	id := mustNewUUID()
	zero := numericFromInt64(0)
	if _, err := q.UpsertAccount(ctx, dbsqlc.UpsertAccountParams{
		ID:      id,
		UserID:  uid,
		Asset:   asset,
		Balance: zero,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, err
	}

	acc2, err := q.GetAccountByUserAsset(ctx, dbsqlc.GetAccountByUserAssetParams{
		UserID: uid,
		Asset:  asset,
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return acc2.ID, nil
}

func negate(n pgtype.Numeric) pgtype.Numeric {
	if n.Int == nil {
		return n
	}
	cp := new(big.Int).Set(n.Int)
	cp.Neg(cp)
	return pgtype.Numeric{Int: cp, Valid: n.Valid, Exp: n.Exp}
}

func (e *Engine) handleCancel(ctx context.Context, id string) (bool, error) {
	orderUUID, err := uuidFromString(id)
	if err != nil {
		log.Printf("handleCancel: invalid order id %s: %v", id, err)
		return false, err
	}

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		log.Printf("handleCancel: begin tx failed for %s: %v", id, err)
		return false, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	qtx := e.queries.WithTx(tx)
	if err := qtx.MarkOrderCancelled(ctx, orderUUID); err != nil {
		log.Printf("handleCancel: mark cancelled failed for %s: %v", id, err)
		return false, err
	}

	e.book.CancelOrder(id)

	if err := tx.Commit(ctx); err != nil {
		log.Printf("handleCancel: commit failed for %s: %v", id, err)
		return false, err
	}
	tx = nil

	return true, nil
}

// Bootstrap reloads resting orders from the database into the in-memory book.
func (e *Engine) Bootstrap(ctx context.Context, market *string) error {
	if e.queries == nil {
		return fmt.Errorf("bootstrap: queries is nil")
	}

	var marketParam string
	if market != nil {
		marketParam = strings.TrimSpace(*market)
	}

	asks, err := e.queries.ListRestingAsks(ctx, marketParam)
	if err != nil {
		return fmt.Errorf("bootstrap asks: %w", err)
	}
	for _, r := range asks {
		o := &Order{
			ID:        uuid.UUID(r.ID.Bytes).String(),
			UserID:    uuid.UUID(r.UserID.Bytes).String(),
			Market:    r.Market,
			Side:      SideSell,
			Price:     numericToInt64(r.Price),
			Quantity:  numericToInt64(r.Quantity),
			Remaining: numericToInt64(r.Remaining),
			IsMarket:  false,
		}
		e.book.AddOrder(o)
	}

	bids, err := e.queries.ListRestingBids(ctx, marketParam)
	if err != nil {
		return fmt.Errorf("bootstrap bids: %w", err)
	}
	for _, r := range bids {
		o := &Order{
			ID:        uuid.UUID(r.ID.Bytes).String(),
			UserID:    uuid.UUID(r.UserID.Bytes).String(),
			Market:    r.Market,
			Side:      SideBuy,
			Price:     numericToInt64(r.Price),
			Quantity:  numericToInt64(r.Quantity),
			Remaining: numericToInt64(r.Remaining),
			IsMarket:  false,
		}
		e.book.AddOrder(o)
	}

	log.Printf("bootstrap loaded %d asks, %d bids into book", len(asks), len(bids))
	return nil
}

func (e *Engine) handlePlace(ctx context.Context, cmd Command) {
	tx, txErr := e.pool.Begin(ctx)
	if txErr != nil {
		log.Printf("handlePlace: begin tx failed for order %s: %v", cmd.Order.ID, txErr)
		cmd.Resp <- placeResult{Result: nil, Err: txErr}
		return
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	res, err := e.matcher.Submit(cmd.Order)
	if err != nil {
		log.Printf("handlePlace: matcher failed for order %s: %v", cmd.Order.ID, err)
		cmd.Resp <- placeResult{Result: res, Err: err}
		return
	}

	qtx := e.queries.WithTx(tx)

	orderUUID, err := uuidFromString(cmd.Order.ID)
	if err != nil {
		log.Printf("handlePlace: invalid order id %s: %v", cmd.Order.ID, err)
		cmd.Resp <- placeResult{Result: res, Err: fmt.Errorf("invalid order id: %w", err)}
		return
	}
	userUUID, err := uuidFromString(cmd.Order.UserID)
	if err != nil {
		log.Printf("handlePlace: invalid user id %s: %v", cmd.Order.UserID, err)
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
		log.Printf("handlePlace: upsert failed for order %s: %v", cmd.Order.ID, err)
		cmd.Resp <- placeResult{Result: res, Err: err}
		return
	}

	if len(res.Trades) > 0 {
		if err := e.persistTradesAndLedger(ctx, qtx, res.Trades); err != nil {
			log.Printf("handlePlace: persistTradesAndLedger failed for order %s: %v", cmd.Order.ID, err)
			cmd.Resp <- placeResult{Result: res, Err: err}
			return
		}
		if err := e.updateMatchedOrders(ctx, qtx, cmd.Order, res.Trades); err != nil {
			log.Printf("handlePlace: updateMatchedOrders failed for order %s: %v", cmd.Order.ID, err)
			cmd.Resp <- placeResult{Result: res, Err: err}
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		log.Printf("handlePlace: commit failed for order %s: %v", cmd.Order.ID, err)
		cmd.Resp <- placeResult{Result: res, Err: err}
		return
	}
	tx = nil

	cmd.Resp <- placeResult{Result: res, Err: nil}
}
