package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	exdb "github.com/hakimelghazi/exchange-core/db"
	dbsqlc "github.com/hakimelghazi/exchange-core/db/sqlc"
	"github.com/hakimelghazi/exchange-core/internal/engine"
)

type placeOrderRequest struct {
	ID       string `json:"id"`      // client-supplied
	UserID   string `json:"user_id"` // later: auth
	Market   string `json:"market"`  // "BTC-USD"
	Side     string `json:"side"`    // "BUY" | "SELL"
	Price    int64  `json:"price"`   // for limit
	Quantity int64  `json:"quantity"`
	IsMarket bool   `json:"is_market"`
}

func main() {
	ctx := context.Background()

	// 1) DB/pool/sqlc
	pool, err := exdb.NewPool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	queries := dbsqlc.New(pool)

	// 2) engine
	eng := engine.NewEngine(1024, pool, queries)
	go eng.Run(ctx)

	// 3) router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// POST /orders
	r.Post("/orders", func(w http.ResponseWriter, r *http.Request) {
		var req placeOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// build engine.Order from request
		side, err := engine.ParseSide(req.Side)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		o := &engine.Order{
			ID:        req.ID,
			UserID:    req.UserID,
			Market:    req.Market,
			Side:      side,
			Price:     req.Price,
			Quantity:  req.Quantity,
			Remaining: req.Quantity,
			IsMarket:  req.IsMarket,
			CreatedAt: time.Now(),
		}

		// send to engine
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		res, placeErr := eng.Place(ctx, o)
		if placeErr != nil {
			http.Error(w, placeErr.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	})

	// DELETE /orders/{id}
	r.Delete("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		ok, cancelErr := eng.Cancel(ctx, id)
		if cancelErr != nil {
			http.Error(w, cancelErr.Error(), http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "order not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /trades?order_id=...
	r.Get("/trades", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.URL.Query().Get("order_id")
		if orderID == "" {
			http.Error(w, "order_id required", http.StatusBadRequest)
			return
		}
		// call generated query
		uuidVal, err := uuid.Parse(orderID)
		if err != nil {
			http.Error(w, "invalid order_id", http.StatusBadRequest)
			return
		}
		pgUUID := pgtype.UUID{Bytes: uuidVal, Valid: true}

		rows, queryErr := queries.ListTradesByOrder(r.Context(), pgUUID)
		if queryErr != nil {
			http.Error(w, queryErr.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rows)
	})

	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}
