package docs

import (
	"errors"
	"fmt"

	"github.com/1broseidon/ketch/config"
)

// ErrUnknownBackend reports a backend name that is not a known docs backend.
// Callers classify errors wrapping it as validation failures; any other
// NewFromConfig error is a missing precondition (API key, unimplemented
// backend).
var ErrUnknownBackend = errors.New("unknown docs backend")

// NewFromConfig builds the docs Searcher for backend, resolving API keys from
// cfg exactly as the `ketch docs` CLI does. Both the CLI and the MCP server
// call this — it is the single owner of the backend switch.
//
// The "local" FTS5 backend is a stub whose Search always errors, so it is
// rejected here — at construction time — instead of failing on first use.
func NewFromConfig(cfg *config.Config, backend string) (Searcher, error) {
	switch backend {
	case "context7":
		if cfg.Context7APIKey == "" {
			return nil, fmt.Errorf("context7: API key not set (get one then: ketch config set context7_api_key <key>)")
		}
		return NewContext7(cfg.Context7APIKey), nil
	case "local":
		return nil, fmt.Errorf("docs backend %q not yet implemented (use context7)", backend)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownBackend, backend)
	}
}
