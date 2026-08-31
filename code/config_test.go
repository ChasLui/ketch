package code

import (
	"errors"
	"strings"
	"testing"

	"github.com/1broseidon/ketch/config"
)

func TestNewFromConfigFirecrawl(t *testing.T) {
	t.Parallel()

	t.Run("requires a key", func(t *testing.T) {
		t.Parallel()
		cfg := config.Defaults()
		_, err := NewFromConfig(&cfg, "firecrawl")
		if err == nil {
			t.Fatal("expected a precondition error without a key")
		}
		// Must not be classified as an unknown backend — the name is valid,
		// the credential is missing.
		if errors.Is(err, ErrUnknownBackend) {
			t.Error("missing key reported as an unknown backend")
		}
		if !strings.Contains(err.Error(), "firecrawl_api_key") {
			t.Errorf("error %q does not say how to set the key", err)
		}
	})

	t.Run("builds with the shared firecrawl key pool", func(t *testing.T) {
		t.Parallel()
		cfg := config.Defaults()
		cfg.FirecrawlAPIKeys = []string{"one", "two"}
		searcher, err := NewFromConfig(&cfg, "firecrawl")
		if err != nil {
			t.Fatalf("NewFromConfig: %v", err)
		}
		backend, ok := searcher.(*Firecrawl)
		if !ok {
			t.Fatalf("got %T, want *Firecrawl", searcher)
		}
		if len(backend.keys) != 2 {
			t.Errorf("key pool has %d keys, want 2", len(backend.keys))
		}
		if want := config.FirecrawlDeveloperSearchURL(config.DefaultFirecrawlURL); backend.endpoint != want {
			t.Errorf("endpoint = %q, want %q", backend.endpoint, want)
		}
	})
}

func TestAvailableCodeBackendsIncludesFirecrawl(t *testing.T) {
	t.Parallel()

	for _, name := range config.AvailableCodeBackends() {
		if name == "firecrawl" {
			return
		}
	}
	t.Error("firecrawl missing from AvailableCodeBackends")
}
