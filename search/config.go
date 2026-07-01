package search

import (
	"errors"
	"fmt"

	"github.com/1broseidon/ketch/config"
)

// ErrUnknownBackend reports a backend name that is not a known search backend.
// Callers classify errors wrapping it as validation failures (bad input);
// any other NewFromConfig error is a missing precondition (e.g. API key).
var ErrUnknownBackend = errors.New("unknown backend")

// NewFromConfig builds the Searcher for backend, resolving API keys and
// instance URLs from cfg exactly as the `ketch search` CLI does. searxngURL
// overrides cfg.SearxngURL when non-empty (the --searxng-url flag / MCP
// searxng_url param). Both the CLI and the MCP server call this — it is the
// single owner of the backend switch.
func NewFromConfig(cfg *config.Config, backend, searxngURL string) (Searcher, error) {
	switch backend {
	case "brave":
		if cfg.BraveAPIKey == "" {
			return nil, fmt.Errorf("brave: API key not set (get one free at https://brave.com/search/api/ then: ketch config set brave_api_key <key>)")
		}
		return NewBrave(cfg.BraveAPIKey), nil
	case "searxng":
		if searxngURL == "" {
			searxngURL = cfg.SearxngURL
		}
		return NewSearXNG(searxngURL), nil
	case "ddg":
		return NewDDG(), nil
	case "exa":
		var apiKey *string
		if cfg.ExaAPIKey != "" {
			apiKey = &cfg.ExaAPIKey
		}
		return NewEXA(apiKey), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownBackend, backend)
	}
}
