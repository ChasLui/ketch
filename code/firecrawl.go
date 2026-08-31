package code

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"

	"github.com/1broseidon/ketch/config"
	"github.com/1broseidon/ketch/httpx"
)

// firecrawlMaxK is the Developer Index's ceiling on k. Asking for more is a
// hard 400, so an over-large --limit is clamped rather than rejected.
const firecrawlMaxK = 100

// firecrawlSnippetMax bounds the joined title+passage markdown, in runes.
const firecrawlSnippetMax = 1200

// Firecrawl searches the Firecrawl Developer Index: issues, merged pull
// requests, READMEs and curated documentation sites, retrieved semantically.
// Unlike the grep-style backends it returns whole artifacts with the passages
// that matched, so results carry no file path or line number.
type Firecrawl struct {
	keys     []string
	client   *http.Client
	endpoint string
}

// NewFirecrawl creates a Developer Index code search backend. baseURL is the
// Firecrawl API base; the developer path is appended.
func NewFirecrawl(keys []string, baseURL string) *Firecrawl {
	return &Firecrawl{
		keys:     keys,
		client:   httpx.Default(),
		endpoint: config.FirecrawlDeveloperSearchURL(baseURL),
	}
}

type firecrawlDevRequest struct {
	Query       string `json:"query"`
	K           int    `json:"k"`
	Language    string `json:"language,omitempty"`
	Integration string `json:"integration,omitempty"`
}

type firecrawlDevResponse struct {
	Success bool `json:"success"`
	// Partial is undocumented upstream; decoded so the field is not silently
	// dropped, but nothing reads it yet.
	Partial bool                 `json:"partial"`
	Results []firecrawlDevResult `json:"results"`
}

type firecrawlDevResult struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	Title    string `json:"title"`
	Passages []struct {
		Text string `json:"text"`
	} `json:"passages"`
}

// Search queries the Developer Index and returns up to q.Limit results.
func (f *Firecrawl) Search(ctx context.Context, q Query) ([]Result, error) {
	// Semantic mismatch first: a regex request is wrong no matter the limit.
	if q.Regexp {
		return nil, ErrRegexpUnsupported
	}
	k := q.Limit
	if k <= 0 {
		return []Result{}, nil
	}
	if k > firecrawlMaxK {
		k = firecrawlMaxK
	}

	body, err := json.Marshal(firecrawlDevRequest{
		Query:       q.Term,
		K:           k,
		Language:    q.Lang,
		Integration: "_ketch",
	})
	if err != nil {
		return nil, err
	}

	// ponytail: key rotation is inlined because search.keyPool is unexported
	// there and this is only the second consumer. Promote to an internal
	// package at the third, not before.
	i := 0
	if len(f.keys) > 1 {
		i = rand.IntN(len(f.keys))
	}
	resp, err := f.post(ctx, body, f.key(i))
	if err != nil {
		return nil, err
	}
	if firecrawlRetryable(resp.StatusCode) && len(f.keys) > 1 {
		drainFirecrawl(resp)
		i++
		if resp, err = f.post(ctx, body, f.key(i)); err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()

	if err := firecrawlStatusErr(resp, f.keyLabel(i)); err != nil {
		return nil, err
	}

	var fr firecrawlDevResponse
	if err := json.NewDecoder(resp.Body).Decode(&fr); err != nil {
		return nil, fmt.Errorf("failed to decode firecrawl developer response: %w", err)
	}

	results := make([]Result, 0, k)
	for _, r := range fr.Results {
		if len(results) >= k {
			break
		}
		if r.URL == "" {
			continue
		}
		results = append(results, firecrawlToResult(r))
	}
	return results, nil
}

func (f *Firecrawl) post(ctx context.Context, body []byte, key string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("firecrawl developer request failed: %w", err)
	}
	return resp, nil
}

// key returns the i-th credential, wrapping around an empty pool to "".
func (f *Firecrawl) key(i int) string {
	if len(f.keys) == 0 {
		return ""
	}
	return f.keys[i%len(f.keys)]
}

// keyLabel identifies a credential without exposing it in an error message.
func (f *Firecrawl) keyLabel(i int) string {
	if len(f.keys) == 0 {
		return "without an API key"
	}
	return fmt.Sprintf("key %d of %d", i%len(f.keys)+1, len(f.keys))
}

func firecrawlRetryable(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusPaymentRequired ||
		status == http.StatusTooManyRequests
}

func drainFirecrawl(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

// firecrawlStatusErr maps a non-200 response to an actionable error. The body
// is echoed for unclassified statuses so upstream detail (an anti-abuse block,
// say) reaches the operator instead of a bare status code.
func firecrawlStatusErr(resp *http.Response, label string) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		return fmt.Errorf("firecrawl developer index: invalid API key (%s; set via: ketch config set firecrawl_api_key <key>)", label)
	case http.StatusPaymentRequired:
		return fmt.Errorf("firecrawl developer index: payment required (%s)", label)
	case http.StatusTooManyRequests:
		return fmt.Errorf("firecrawl developer index: rate limited (%s)", label)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if detail := strings.TrimSpace(string(body)); detail != "" {
		return fmt.Errorf("firecrawl developer index returned status %d: %s", resp.StatusCode, detail)
	}
	return fmt.Errorf("firecrawl developer index returned status %d", resp.StatusCode)
}

// firecrawlToResult maps one Developer Index artifact onto a code.Result.
// The artifact kind lives in the id prefix; Line, Stars and Language stay zero
// because the index does not return them.
func firecrawlToResult(r firecrawlDevResult) Result {
	repo, path := firecrawlRepoPath(r.ID, r.URL)
	return Result{
		Repo:    repo,
		Path:    path,
		Snippet: firecrawlSnippet(r),
		URL:     r.URL,
		Source:  "firecrawl",
	}
}

// firecrawlRepoPath splits an artifact id such as "issue:owner/repo#123" or
// "doc:https://example.com/page" into a repo identity and a path-ish label.
// Repository-backed kinds yield the owner/repo pair; documentation and web
// artifacts fall back to the URL host so the CLI's "repo  path" header and the
// --minimal repo column stay meaningful.
func firecrawlRepoPath(id, rawURL string) (repo, path string) {
	// Cut on the first colon only: doc:/web: ids embed a full URL.
	kind, rest, found := strings.Cut(id, ":")
	if !found {
		return firecrawlHost(rawURL), id
	}
	switch kind {
	case "readme":
		return rest, "readme"
	case "issue", "pull_request":
		owner, number, ok := strings.Cut(rest, "#")
		if !ok {
			return rest, kind
		}
		return owner, kind + "#" + number
	case "doc", "web":
		return firecrawlHost(rawURL), kind
	default:
		return firecrawlHost(rawURL), id
	}
}

func firecrawlHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// firecrawlSnippet joins the title with every matched passage. Passage
// markdown is kept intact — tables and code blocks are the point of this
// backend — and the CLI's --minimal mode takes only the first line, so the
// one-result-per-line contract holds without flattening.
func firecrawlSnippet(r firecrawlDevResult) string {
	parts := make([]string, 0, len(r.Passages)+1)
	if head := strings.TrimSpace(r.Title); head != "" {
		parts = append(parts, head)
	} else if r.URL != "" {
		parts = append(parts, r.URL)
	}
	for _, p := range r.Passages {
		if text := strings.TrimSpace(p.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return truncateRunes(strings.Join(parts, "\n\n"), firecrawlSnippetMax)
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
