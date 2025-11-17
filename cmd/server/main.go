package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"gopkg.in/yaml.v3"

	exdb "github.com/hakimelghazi/exchange-core/db"
	dbsqlc "github.com/hakimelghazi/exchange-core/db/sqlc"
	"github.com/hakimelghazi/exchange-core/internal/engine"
)

var (
	openAPIDocYAML []byte
	openAPIDocJSON []byte
	openAPILoadErr error
)

func init() {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		openAPILoadErr = errors.New("unable to determine caller for openapi")
		log.Printf("openapi load error: %v", openAPILoadErr)
		return
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..", "openapi.yaml")
	data, err := os.ReadFile(filepath.Clean(root))
	if err != nil {
		openAPILoadErr = err
		log.Printf("openapi load error: %v", openAPILoadErr)
		return
	}
	openAPIDocYAML = data

	var spec any
	if err := yaml.Unmarshal(data, &spec); err != nil {
		openAPILoadErr = fmt.Errorf("parse openapi yaml: %w", err)
		log.Printf("openapi load error: %v", openAPILoadErr)
		return
	}
	openAPIDocJSON, err = json.Marshal(spec)
	if err != nil {
		openAPILoadErr = fmt.Errorf("marshal openapi json: %w", err)
		log.Printf("openapi load error: %v", openAPILoadErr)
		return
	}
}

type Server struct {
	engine  *engine.Engine
	queries *dbsqlc.Queries
}

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
	eng, err := engine.NewEngine(1024, pool, queries)
	if err != nil {
		log.Fatal(err)
	}
	go eng.Run(ctx)

	// 3) router
	r := chi.NewRouter()

	// Hygiene stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(3 * time.Second))

	server := &Server{
		engine:  eng,
		queries: queries,
	}

	// Read endpoints
	r.Get("/orders/{id}", server.handleGetOrderByID)
	r.Get("/orders", server.handleListOrders)
	r.Get("/trades", server.handleListTrades)
	r.Get("/balances", server.handleGetBalances)
	r.Get("/openapi.yaml", server.handleOpenAPIYAML)
	r.Get("/openapi.json", server.handleOpenAPIJSON)
	r.Get("/docs", server.handleDocs)

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

		// ensure user exists
		userUUID, _ := uuid.Parse(order.UserID)
		if err := ensureUser(r.Context(), queries, pgtype.UUID{Bytes: userUUID, Valid: true}); err != nil {
			writeProblem(w, r, http.StatusInternalServerError, "engine_error", err.Error())
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

func ensureUser(ctx context.Context, q *dbsqlc.Queries, id pgtype.UUID) error {
	if uuid.UUID(id.Bytes) == (uuid.UUID{}) {
		return errors.New("user id required")
	}
	return q.UpsertUser(ctx, dbsqlc.UpsertUserParams{
		ID:    id,
		Email: pgtype.Text{String: fmt.Sprintf("%s@example.com", id.String()), Valid: true},
	})
}

// ---------- read handlers ----------

func (s *Server) handleGetOrderByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := chi.URLParam(r, "id")
	uid, err := uuid.Parse(idStr)
	if err != nil {
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid order id", err.Error())
		return
	}
	row, err := s.queries.GetOrder(ctx, pgUUIDFrom(uid))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, r, http.StatusNotFound, "order not found", err.Error())
		} else {
			writeProblem(w, r, http.StatusInternalServerError, "db_error", err.Error())
		}
		return
	}
	writeJSON(w, r, http.StatusOK, row)
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	userID := parseUUIDPtr(query.Get("user_id"))
	status := parseTextPtr(query.Get("status"))
	side := parseTextPtr(query.Get("side"))
	limit := parseLimit(query.Get("limit"), 50, 500)

	var afterTS pgtype.Timestamptz
	var afterID pgtype.UUID
	if cur := query.Get("cursor"); cur != "" {
		ts, id, err := decodeCursor(cur)
		if err != nil {
			writeProblem(w, r, http.StatusUnprocessableEntity, "invalid cursor", err.Error())
			return
		}
		afterTS = pgTimestamptzFrom(ts)
		afterID = pgUUIDFrom(id)
	}

	params := dbsqlc.ListOrdersParams{
		Column1: pgUUIDFromPtr(userID),
		Column2: textParam(status),
		Column3: textParam(side),
		Column4: afterTS,
		Column5: afterID,
		Limit:   int32(limit),
	}

	rows, err := s.queries.ListOrders(ctx, params)
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	var nextCursor *string
	if limit > 0 && len(rows) == int(limit) {
		last := rows[len(rows)-1]
		ts := time.Now().UTC()
		if last.CreatedAt.Valid {
			ts = last.CreatedAt.Time
		}
		cur := encodeCursor(ts, uuid.UUID(last.ID.Bytes))
		nextCursor = &cur
	}

	resp := struct {
		Items      any     `json:"items"`
		NextCursor *string `json:"next_cursor,omitempty"`
	}{
		Items:      rows,
		NextCursor: nextCursor,
	}
	writeJSON(w, r, http.StatusOK, resp)
}

func (s *Server) handleListTrades(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	userID := parseUUIDPtr(query.Get("user_id"))
	orderID := parseUUIDPtr(query.Get("order_id"))
	market := strings.TrimSpace(query.Get("market"))
	limit := parseLimit(query.Get("limit"), 100, 1000)

	var since pgtype.Timestamptz
	if sinceRaw := query.Get("since"); sinceRaw != "" {
		t, err := time.Parse(time.RFC3339Nano, sinceRaw)
		if err != nil {
			writeProblem(w, r, http.StatusUnprocessableEntity, "invalid since", err.Error())
			return
		}
		since = pgTimestamptzFrom(t)
	}

	rows, err := s.queries.ListTrades(ctx, dbsqlc.ListTradesParams{
		Column1: pgUUIDFromPtr(userID),
		Column2: pgUUIDFromPtr(orderID),
		Column3: market,
		Column4: since,
		Limit:   int32(limit),
	})
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	writeJSON(w, r, http.StatusOK, struct {
		Items any `json:"items"`
	}{Items: rows})
}

func (s *Server) handleGetBalances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uidStr := r.URL.Query().Get("user_id")
	if uidStr == "" {
		writeProblem(w, r, http.StatusUnprocessableEntity, "missing user_id", "")
		return
	}
	uid, err := uuid.Parse(uidStr)
	if err != nil {
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid user_id", err.Error())
		return
	}
	rows, err := s.queries.GetBalancesByUser(ctx, pgUUIDFrom(uid))
	if err != nil {
		writeProblem(w, r, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, rows)
}

func (s *Server) handleOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	if openAPILoadErr != nil {
		log.Printf("handleOpenAPIYAML: %v", openAPILoadErr)
		writeProblem(w, r, http.StatusInternalServerError, "openapi_unavailable", openAPILoadErr.Error())
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPIDocYAML)
}

func (s *Server) handleOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	if openAPILoadErr != nil {
		log.Printf("handleOpenAPIJSON: %v", openAPILoadErr)
		writeProblem(w, r, http.StatusInternalServerError, "openapi_unavailable", openAPILoadErr.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(openAPIDocJSON)
}

func (s *Server) handleDocs(w http.ResponseWriter, r *http.Request) {
	const html = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>exchange-core API docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  </head>
  <body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
      window.onload = () => {
        SwaggerUIBundle({
          url: '/openapi.json',
          dom_id: '#swagger-ui'
        });
      };
    </script>
  </body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, r *http.Request, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	if rid := middleware.GetReqID(r.Context()); rid != "" {
		w.Header().Set("X-Request-ID", rid)
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeProblem(w http.ResponseWriter, r *http.Request, code int, title, detail string) {
	reqID := middleware.GetReqID(r.Context())
	w.Header().Set("Content-Type", "application/problem+json")
	if reqID != "" {
		w.Header().Set("X-Request-ID", reqID)
	}
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":      title,
		"status":     code,
		"detail":     detail,
		"instance":   r.URL.Path,
		"request_id": reqID,
	})
}

func parseLimit(raw string, def, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

func parseUUIDPtr(s string) *uuid.UUID {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &u
}

func parseTextPtr(s string) pgtype.Text {
	t := strings.TrimSpace(s)
	if t == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: t, Valid: true}
}

func textParam(t pgtype.Text) interface{} {
	if !t.Valid {
		return nil
	}
	return t.String
}

func pgUUIDFrom(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}

func pgUUIDFromPtr(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{Valid: false}
	}
	return pgUUIDFrom(*u)
}

func pgTimestamptzFrom(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t.UTC(), Valid: true}
}

type cursorPayload struct {
	TS time.Time `json:"ts"`
	ID uuid.UUID `json:"id"`
}

func encodeCursor(ts time.Time, id uuid.UUID) string {
	b, _ := json.Marshal(cursorPayload{TS: ts.UTC(), ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(cur string) (time.Time, uuid.UUID, error) {
	data, err := base64.RawURLEncoding.DecodeString(cur)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	var payload cursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return time.Time{}, uuid.Nil, err
	}
	return payload.TS, payload.ID, nil
}
