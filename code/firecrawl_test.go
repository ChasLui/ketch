package code

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// newTestFirecrawl builds a backend pointed straight at a test server, skipping
// the config-driven endpoint join.
func newTestFirecrawl(t *testing.T, keys []string, h http.HandlerFunc) *Firecrawl {
	t.Helper()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	return &Firecrawl{keys: keys, client: server.Client(), endpoint: server.URL}
}

func writeDevResults(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func TestFirecrawlMapsEveryArtifactKind(t *testing.T) {
	t.Parallel()

	const body = `{"success":true,"partial":false,"results":[
		{"id":"readme:hashicorp/go-retryablehttp","url":"https://github.com/hashicorp/go-retryablehttp","title":"hashicorp/go-retryablehttp","passages":[{"text":"# Example Use"}]},
		{"id":"issue:photoprism/photoprism#5732","url":"https://github.com/photoprism/photoprism/issues/5732","title":"Add a shared outbound client","passages":[{"text":"retry"}]},
		{"id":"pull_request:langchain-ai/open-swe#1565","url":"https://github.com/langchain-ai/open-swe/issues/1565","title":"langchain-ai/open-swe#1565","passages":[{"text":"patch"}]},
		{"id":"doc:https://docs.cypress.io/app/guides/test-retries","url":"https://docs.cypress.io/app/guides/test-retries","title":"Test Retries","passages":[{"text":"## Configure"}]},
		{"id":"web:https://tutorialedge.net/golang/retrying/","url":"https://tutorialedge.net/golang/retrying/","title":"Retrying HTTP Requests","passages":[{"text":"Delay option"}]},
		{"id":"mystery-no-colon","url":"https://example.com/thing","title":"Unknown","passages":[]}
	]}`

	f := newTestFirecrawl(t, []string{"fc-test"}, func(w http.ResponseWriter, _ *http.Request) {
		writeDevResults(t, w, body)
	})

	results, err := f.Search(context.Background(), Query{Term: "retries", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 6 {
		t.Fatalf("got %d results, want 6", len(results))
	}

	want := []struct{ repo, path string }{
		{"hashicorp/go-retryablehttp", "readme"},
		{"photoprism/photoprism", "issue#5732"},
		{"langchain-ai/open-swe", "pull_request#1565"},
		{"docs.cypress.io", "doc"},
		{"tutorialedge.net", "web"},
		{"example.com", "mystery-no-colon"},
	}
	for i, w := range want {
		if results[i].Repo != w.repo {
			t.Errorf("result %d repo = %q, want %q", i, results[i].Repo, w.repo)
		}
		if results[i].Path != w.path {
			t.Errorf("result %d path = %q, want %q", i, results[i].Path, w.path)
		}
		if results[i].Source != "firecrawl" {
			t.Errorf("result %d source = %q, want firecrawl", i, results[i].Source)
		}
		// The Developer Index returns none of these; they must stay zero
		// rather than being inferred from the query.
		if results[i].Line != 0 || results[i].Stars != 0 || results[i].Language != "" {
			t.Errorf("result %d has phantom metadata: line=%d stars=%d lang=%q",
				i, results[i].Line, results[i].Stars, results[i].Language)
		}
	}
}

func TestFirecrawlRequestShape(t *testing.T) {
	t.Parallel()

	var got firecrawlDevRequest
	var auth string
	f := newTestFirecrawl(t, []string{"fc-test"}, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writeDevResults(t, w, `{"success":true,"results":[]}`)
	})

	if _, err := f.Search(context.Background(), Query{Term: "http retries", Lang: "Go", Limit: 3}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := firecrawlDevRequest{Query: "http retries", K: 3, Language: "Go", Integration: "_ketch"}
	if got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
	if auth != "Bearer fc-test" {
		t.Errorf("Authorization = %q, want %q", auth, "Bearer fc-test")
	}
}

func TestFirecrawlOmitsEmptyLanguage(t *testing.T) {
	t.Parallel()

	var raw map[string]any
	f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writeDevResults(t, w, `{"success":true,"results":[]}`)
	})

	if _, err := f.Search(context.Background(), Query{Term: "x", Limit: 1}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if _, ok := raw["language"]; ok {
		t.Errorf("language present in request body with empty Lang: %v", raw)
	}
}

func TestFirecrawlClampsLimitToMaxK(t *testing.T) {
	t.Parallel()

	var got firecrawlDevRequest
	f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writeDevResults(t, w, `{"success":true,"results":[]}`)
	})

	// Upstream rejects k > 100 with a hard 400, so an over-large limit must be
	// clamped rather than passed through.
	if _, err := f.Search(context.Background(), Query{Term: "x", Limit: 500}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if got.K != firecrawlMaxK {
		t.Errorf("k = %d, want %d", got.K, firecrawlMaxK)
	}
}

func TestFirecrawlRespectsLimit(t *testing.T) {
	t.Parallel()

	const body = `{"success":true,"results":[
		{"id":"readme:a/b","url":"https://github.com/a/b","title":"b"},
		{"id":"readme:c/d","url":"https://github.com/c/d","title":"d"},
		{"id":"readme:e/f","url":"https://github.com/e/f","title":"f"}
	]}`
	f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
		writeDevResults(t, w, body)
	})

	results, err := f.Search(context.Background(), Query{Term: "x", Limit: 2})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestFirecrawlSkipsRequestWithoutWork(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		query   Query
		wantErr error
	}{
		{"zero limit", Query{Term: "x", Limit: 0}, nil},
		{"negative limit", Query{Term: "x", Limit: -3}, nil},
		{"regexp unsupported", Query{Term: "x", Limit: 5, Regexp: true}, ErrRegexpUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFirecrawl(t, []string{"k"}, func(_ http.ResponseWriter, _ *http.Request) {
				t.Error("backend must not issue a request")
			})
			results, err := f.Search(context.Background(), tc.query)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(results) != 0 {
				t.Errorf("got %d results, want 0", len(results))
			}
		})
	}
}

func TestFirecrawlCredentialErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, "firecrawl_api_key"},
		{"payment required", http.StatusPaymentRequired, "payment required"},
		{"rate limited", http.StatusTooManyRequests, "rate limited"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			})
			_, err := f.Search(context.Background(), Query{Term: "x", Limit: 1})
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
			if !strings.Contains(err.Error(), "key 1 of 1") {
				t.Errorf("error %q does not label the credential", err)
			}
		})
	}
}

func TestFirecrawlEchoesUnclassifiedStatusBody(t *testing.T) {
	t.Parallel()

	f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		writeDevResults(t, w, `{"success":false,"error":"your IP address looks suspicious"}`)
	})

	_, err := f.Search(context.Background(), Query{Term: "x", Limit: 1})
	if err == nil {
		t.Fatal("expected an error")
	}
	// The operator can only act on this if the upstream detail survives.
	if !strings.Contains(err.Error(), "suspicious") {
		t.Errorf("error %q drops the upstream detail", err)
	}
}

func TestFirecrawlRotatesKeyOnCredentialFailure(t *testing.T) {
	t.Parallel()

	var seen []string
	f := newTestFirecrawl(t, []string{"first", "second"}, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if len(seen) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeDevResults(t, w, `{"success":true,"results":[{"id":"readme:a/b","url":"https://github.com/a/b","title":"b"}]}`)
	})

	results, err := f.Search(context.Background(), Query{Term: "x", Limit: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(seen) != 2 {
		t.Fatalf("got %d requests, want 2", len(seen))
	}
	if seen[0] == seen[1] {
		t.Errorf("retry reused the same credential: %q", seen[0])
	}
	if len(results) != 1 {
		t.Errorf("got %d results, want 1", len(results))
	}
}

func TestFirecrawlSnippetComposition(t *testing.T) {
	t.Parallel()

	const body = `{"success":true,"results":[
		{"id":"readme:a/b","url":"https://github.com/a/b","title":"Title here","passages":[{"text":"first"},{"text":"second"}]},
		{"id":"doc:https://example.com/p","url":"https://example.com/p","title":"","passages":[{"text":"only passage"}]},
		{"id":"readme:c/d","url":"https://github.com/c/d","title":"Bare title","passages":[]},
		{"id":"readme:e/f","url":"","title":"dropped"}
	]}`
	f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
		writeDevResults(t, w, body)
	})

	results, err := f.Search(context.Background(), Query{Term: "x", Limit: 10})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// The entry with an empty url carries nothing actionable and is dropped.
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if want := "Title here\n\nfirst\n\nsecond"; results[0].Snippet != want {
		t.Errorf("snippet = %q, want %q", results[0].Snippet, want)
	}
	// A missing title must fall back to the URL rather than leaving the first
	// line — which is all --minimal shows — empty.
	if want := "https://example.com/p\n\nonly passage"; results[1].Snippet != want {
		t.Errorf("snippet = %q, want %q", results[1].Snippet, want)
	}
	if want := "Bare title"; results[2].Snippet != want {
		t.Errorf("snippet = %q, want %q", results[2].Snippet, want)
	}
}

func TestFirecrawlTruncatesLongSnippet(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("é", firecrawlSnippetMax*2)
	body, err := json.Marshal(map[string]any{
		"success": true,
		"results": []map[string]any{{
			"id": "readme:a/b", "url": "https://github.com/a/b", "title": "t",
			"passages": []map[string]string{{"text": long}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
		writeDevResults(t, w, string(body))
	})

	results, err := f.Search(context.Background(), Query{Term: "x", Limit: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// Counted in runes, not bytes — the payload is multi-byte on purpose.
	if got := utf8.RuneCountInString(results[0].Snippet); got != firecrawlSnippetMax+1 {
		t.Errorf("snippet is %d runes, want %d (cap plus ellipsis)", got, firecrawlSnippetMax+1)
	}
	if !strings.HasSuffix(results[0].Snippet, "…") {
		t.Error("truncated snippet does not end in an ellipsis")
	}
}

func TestFirecrawlEmptyAndMalformedResponses(t *testing.T) {
	t.Parallel()

	t.Run("empty results", func(t *testing.T) {
		t.Parallel()
		f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
			writeDevResults(t, w, `{"success":true,"results":[]}`)
		})
		results, err := f.Search(context.Background(), Query{Term: "x", Limit: 5})
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("got %d results, want 0", len(results))
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		t.Parallel()
		f := newTestFirecrawl(t, []string{"k"}, func(w http.ResponseWriter, _ *http.Request) {
			writeDevResults(t, w, `{"success":true,"results":[`)
		})
		if _, err := f.Search(context.Background(), Query{Term: "x", Limit: 5}); err == nil {
			t.Fatal("expected a decode error")
		}
	})
}
