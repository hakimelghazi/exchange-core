package main

import (
	"context"
	"log"

	exdb "github.com/hakimelghazi/exchange-core/db"        // your db/db.go
	dbsqlc "github.com/hakimelghazi/exchange-core/db/sqlc" // generated
	"github.com/hakimelghazi/exchange-core/internal/engine"
)

func main() {
	ctx := context.Background()

	// 1) connect to Postgres
	pool, err := exdb.NewPool(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	// 2) build sqlc Queries
	queries := dbsqlc.New(pool)

	// 3) build engine with queries
	eng := engine.NewEngine(1024, pool, queries) // you’ll tweak the ctor to accept queries

	// 4) start engine loop (and http later)
	go eng.Run(ctx)

	// block / start http...
}
