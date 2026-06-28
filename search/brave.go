package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/1broseidon/ketch/httpx"
)

// Brave searches via the Brave Search API.
type Brave struct {
	apiKey string
	client *http.Client
}

// NewBrave creates a new Brave search backend.
func NewBrave(apiKey string) *Brave {
	return &Brave{
		apiKey: apiKey,
		client: httpx.Default(),
	}
}

type braveResponse struct {
	Web struct {
		Results []braveResult `json:"results"`
	} `json:"web"`
}

type braveResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

const braveMaxCount = 20

// Search queries Brave and returns up to limit results.
func (b *Brave) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	count := braveRequestCount(limit)
	if count == 0 {
		return []Result{}, nil
	}

	u := fmt.Sprintf("https://api.search.brave.com/res/v1/web/search?q=%s&count=%d&text_decorations=false&result_filter=web",
		url.QueryEscape(query), count)

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brave request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("brave: invalid API key (set via: ketch config set brave_api_key <key>)")
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("brave: rate limited")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, braveStatusError(resp)
	}

	var br braveResponse
	if err := json.NewDecoder(resp.Body).Decode(&br); err != nil {
		return nil, fmt.Errorf("failed to decode brave response: %w", err)
	}

	results := make([]Result, 0, count)
	for _, r := range br.Web.Results {
		if len(results) >= count {
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

func braveRequestCount(limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit > braveMaxCount {
		return braveMaxCount
	}
	return limit
}

func braveStatusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if detail := strings.TrimSpace(string(body)); detail != "" {
		return fmt.Errorf("brave returned status %d: %s", resp.StatusCode, detail)
	}
	return fmt.Errorf("brave returned status %d", resp.StatusCode)
}
