// internal/engine/loop.go
package engine

import (
	"context"
	"log"
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
				// 1) match in memory
				res, err := e.matcher.Submit(cmd.Order)

				// 2) persist trades (if any) in one DB tx
				if err == nil && len(res.Trades) > 0 && e.pool != nil && e.queries != nil {
					tx, txErr := e.pool.Begin(ctx)
					if txErr != nil {
						// if we can't write trades, log & return the match result anyway
						log.Printf("begin tx failed: %v", txErr)
					} else {
						qtx := e.queries.WithTx(tx) // sqlc pattern :contentReference[oaicite:2]{index=2}
						persistErr := e.persistTrades(ctx, qtx, res.Trades)
						if persistErr != nil {
							_ = tx.Rollback(ctx)
							log.Printf("persist trades failed: %v", persistErr)
						} else {
							if err := tx.Commit(ctx); err != nil {
								log.Printf("commit failed: %v", err)
							}
						}
					}
				}

				// 3) answer the caller
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

// persistTrades inserts each trade with sqlc
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
