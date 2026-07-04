package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/1broseidon/ketch/httpx"
)

// keenableBaseURL is the Keenable API root. Search hits /v1/search/public
// (keyless) or /v1/search (keyed); the endpoint is chosen by whether a key is
// configured.
const keenableBaseURL = "https://api.keenable.ai"

// keenableTitle is the attribution tag Keenable segments integration traffic
// by. Sent on every request so ketch usage is visible in adoption dashboards.
const keenableTitle = "Ketch"

// Keenable searches via the Keenable web search API, a search index built for
// AI agents. It is keyless by default (rate-limited); an optional API key lifts
// the hourly cap and switches to the authenticated endpoint.
type Keenable struct {
	apiKey *string
	client *http.Client
}

// NewKeenable creates a new Keenable search backend. A nil or blank apiKey uses
// the keyless public endpoint.
func NewKeenable(apiKey *string) *Keenable {
	return &Keenable{
		apiKey: apiKey,
		client: httpx.Default(),
	}
}

type keenableResponse struct {
	Results []keenableResult `json:"results"`
}

type keenableResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// Search queries Keenable and returns up to limit results.
func (k *Keenable) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		return []Result{}, nil
	}

	body, err := json.Marshal(map[string]any{"query": query, "mode": "pro"})
	if err != nil {
		return nil, err
	}

	// Keyless by default; a configured key switches to the authenticated path.
	path := "/v1/search/public"
	key := ""
	if k.apiKey != nil {
		key = strings.TrimSpace(*k.apiKey)
	}
	if key != "" {
		path = "/v1/search"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", keenableBaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "keenable-ketch")
	req.Header.Set("X-Keenable-Title", keenableTitle)
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("keenable request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("keenable: invalid API key (set via: ketch config set keenable_api_key <key>)")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("keenable: rate limited (set a key to lift the cap: ketch config set keenable_api_key <key>)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, keenableStatusError(resp)
	}

	var kr keenableResponse
	if err := json.NewDecoder(resp.Body).Decode(&kr); err != nil {
		return nil, fmt.Errorf("failed to decode keenable response: %w", err)
	}

	results := make([]Result, 0, limit)
	for _, r := range kr.Results {
		if len(results) >= limit {
			break
		}
		results = append(results, Result{
			Title:       r.Title,
			URL:         r.URL,
			Description: r.Description,
		})
	}

	return results, nil
}

func keenableStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if detail := strings.TrimSpace(string(body)); detail != "" {
		return fmt.Errorf("keenable returned status %d: %s", resp.StatusCode, detail)
	}
	return fmt.Errorf("keenable returned status %d", resp.StatusCode)
}
