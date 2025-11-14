package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
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

	// Hygiene stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(3 * time.Second))

	writeProblem := func(w http.ResponseWriter, r *http.Request, code int, title, detail string) {
		reqID := middleware.GetReqID(r.Context())
		w.Header().Set("Content-Type", "application/problem+json")
		w.Header().Set("X-Request-ID", reqID)
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"title":      title,
			"status":     code,
			"detail":     detail,
			"instance":   r.URL.Path,
			"request_id": reqID,
		})
	}

	// POST /orders
	r.Post("/orders", func(w http.ResponseWriter, r *http.Request) {
		var req placeOrderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid_json", err.Error())
			return
		}

		// build engine.Order from request
		order, err := toEngineOrder(req)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "validation_error", err.Error())
			return
		}

		// send to engine using per-request context (timeout middleware already applied)
		res, placeErr := eng.Place(r.Context(), order)
		if placeErr != nil {
			writeProblem(w, r, http.StatusInternalServerError, "engine_error", placeErr.Error())
			return
		}

		rid := middleware.GetReqID(r.Context())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/orders/"+req.ID)
		w.Header().Set("X-Request-ID", rid)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(toOrderCreateResponse(req, res, rid))
	})

	// DELETE /orders/{id}
	r.Delete("/orders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		ok, cancelErr := eng.Cancel(r.Context(), id)
		if cancelErr != nil {
			writeProblem(w, r, http.StatusInternalServerError, "engine_error", cancelErr.Error())
			return
		}
		if !ok {
			writeProblem(w, r, http.StatusNotFound, "not_found", "order not found")
			return
		}
		w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	})

	// GET /trades?order_id=...
	r.Get("/trades", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.URL.Query().Get("order_id")
		if orderID == "" {
			writeProblem(w, r, http.StatusBadRequest, "validation_error", "order_id required")
			return
		}
		// call generated query
		uuidVal, err := uuid.Parse(orderID)
		if err != nil {
			writeProblem(w, r, http.StatusBadRequest, "validation_error", "invalid order_id")
			return
		}
		pgUUID := pgtype.UUID{Bytes: uuidVal, Valid: true}

		rows, queryErr := queries.ListTradesByOrder(r.Context(), pgUUID)
		if queryErr != nil {
			writeProblem(w, r, http.StatusInternalServerError, "db_error", queryErr.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", middleware.GetReqID(r.Context()))
		_ = json.NewEncoder(w).Encode(rows)
	})

	log.Println("listening on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatal(err)
	}
}

func toEngineOrder(req placeOrderRequest) (*engine.Order, error) {
	req.ID = strings.TrimSpace(req.ID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.Market = strings.TrimSpace(req.Market)
	req.Side = strings.TrimSpace(req.Side)

	if req.ID == "" || req.UserID == "" || req.Market == "" {
		return nil, errors.New("id, user_id, and market are required")
	}
	if _, err := uuid.Parse(req.ID); err != nil {
		return nil, errors.New("id must be a valid uuid")
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		return nil, errors.New("user_id must be a valid uuid")
	}
	if req.Quantity <= 0 {
		return nil, errors.New("quantity must be positive")
	}
	if !req.IsMarket && req.Price <= 0 {
		return nil, errors.New("limit orders require positive price")
	}

	side, err := engine.ParseSide(req.Side)
	if err != nil {
		return nil, err
	}

	return &engine.Order{
		ID:        req.ID,
		UserID:    req.UserID,
		Market:    req.Market,
		Side:      side,
		Price:     req.Price,
		Quantity:  req.Quantity,
		Remaining: req.Quantity,
		IsMarket:  req.IsMarket,
		CreatedAt: time.Now(),
	}, nil
}

type orderCreateResponse struct {
	OrderID    string         `json:"order_id"`
	UserID     string         `json:"user_id"`
	Market     string         `json:"market"`
	Side       string         `json:"side"`
	Quantity   int64          `json:"quantity"`
	Filled     bool           `json:"filled"`
	Remaining  int64          `json:"remaining"`
	Resting    bool           `json:"resting"`
	Trades     []engine.Trade `json:"trades"`
	RequestID  string         `json:"request_id"`
	ReceivedAt time.Time      `json:"received_at"`
}

func toOrderCreateResponse(req placeOrderRequest, res *engine.MatchResult, requestID string) orderCreateResponse {
	remaining := int64(0)
	if res.Remainder != nil {
		remaining = res.Remainder.Remaining
	}
	return orderCreateResponse{
		OrderID:    req.ID,
		UserID:     req.UserID,
		Market:     req.Market,
		Side:       strings.ToUpper(req.Side),
		Quantity:   req.Quantity,
		Filled:     res.OrderFilled,
		Remaining:  remaining,
		Resting:    res.Remainder != nil && !req.IsMarket,
		Trades:     res.Trades,
		RequestID:  requestID,
		ReceivedAt: time.Now().UTC(),
	}
}
