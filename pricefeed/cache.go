package pricefeed

import (
	"context"
	"log"
	"sync"
	"time"
)

// PriceCache stores latest prices for markets in memory.
type PriceCache struct {
	mu     sync.RWMutex
	prices map[string]float64
}

func NewPriceCache() *PriceCache {
	return &PriceCache{prices: make(map[string]float64)}
}

func (c *PriceCache) Set(market string, price float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prices[market] = price
}

func (c *PriceCache) Get(market string) (float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.prices[market]
	return p, ok
}

// StartPriceUpdater periodically refreshes prices for the given markets.
func StartPriceUpdater(
	ctx context.Context,
	feed PriceFeed,
	cache *PriceCache,
	markets []string,
	interval time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	refreshOnce(ctx, feed, cache, markets)

	for {
		select {
		case <-ticker.C:
			refreshOnce(ctx, feed, cache, markets)
		case <-ctx.Done():
			return
		}
	}
}

func refreshOnce(ctx context.Context, feed PriceFeed, cache *PriceCache, markets []string) {
	for _, m := range markets {
		price, err := feed.GetSpot(ctx, m)
		if err != nil {
			log.Printf("price update failed for %s: %v", m, err)
			continue
		}
		cache.Set(m, price)
		log.Printf("price update: %s %.2f", m, price)
	}
}
