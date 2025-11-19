package pricefeed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type PriceFeed interface {
	GetSpot(ctx context.Context, market string) (float64, error)
}

// CoinGeckoFeed implements PriceFeed using the public CoinGecko API.
type CoinGeckoFeed struct {
	client  *http.Client
	baseURL string
}

// NewCoinGeckoFeed returns a new CoinGecko-based price feed.
func NewCoinGeckoFeed() *CoinGeckoFeed {
	return &CoinGeckoFeed{
		client: &http.Client{Timeout: 5 * time.Second},
		baseURL: "https://api.coingecko.com/api/v3",
	}
}

// internal: map our markets ("BTC-USD") to CoinGecko ids ("bitcoin").
func mapMarketToCoinGeckoID(market string) (string, error) {
	switch market {
	case "BTC-USD":
		return "bitcoin", nil
	case "ETH-USD":
		return "ethereum", nil
	default:
		return "", fmt.Errorf("unsupported market: %s", market)
	}
}

type cgResponse map[string]struct {
	USD float64 `json:"usd"`
}

// GetSpot returns the spot price in USD for the given market (e.g. "BTC-USD").
func (f *CoinGeckoFeed) GetSpot(ctx context.Context, market string) (float64, error) {
	id, err := mapMarketToCoinGeckoID(market)
	if err != nil {
		return 0, err
	}

	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=usd", f.baseURL, id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("coingecko: unexpected status %d", resp.StatusCode)
	}

	var body cgResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, err
	}

	entry, ok := body[id]
	if !ok {
		return 0, fmt.Errorf("coingecko: no price for %s", id)
	}

	return entry.USD, nil
}

