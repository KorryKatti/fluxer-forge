package fluxer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Discovery is what an instance publishes at /.well-known/fluxer.
// We only read the two endpoints the bot needs; the rest is ignored.
type Discovery struct {
	Endpoints struct {
		APIPublic string `json:"api_public"` // e.g. "https://api.fluxer.app"
		Gateway   string `json:"gateway"`    // e.g. "wss://gateway.fluxer.app"
	} `json:"endpoints"`
}

// Discover fetches instanceURL + "/.well-known/fluxer".
// Follows redirects (fluxer.app redirects to api.fluxer.app).
func Discover(ctx context.Context, instanceURL string) (*Discovery, error) {
	url := strings.TrimRight(instanceURL, "/") + "/.well-known/fluxer"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discover %s: HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("discover %s: read body: %w", url, err)
	}

	var discovery Discovery
	if err := json.Unmarshal(body, &discovery); err != nil {
		return nil, fmt.Errorf("discover %s: parse JSON: %w", url, err)
	}

	if discovery.Endpoints.APIPublic == "" || discovery.Endpoints.Gateway == "" {
		return nil, fmt.Errorf("discover %s: response is missing endpoints.api_public or endpoints.gateway", url)
	}

	return &discovery, nil
}
