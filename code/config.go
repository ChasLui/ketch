package code

import (
	"errors"
	"fmt"
	"strings"

	"github.com/1broseidon/ketch/config"
)

// ErrUnknownBackend reports a backend name that is not a known code search
// backend. Callers classify errors wrapping it as validation failures; any
// other NewFromConfig error is a missing precondition (e.g. token).
var ErrUnknownBackend = errors.New("unknown code backend")

// NewFromConfig builds the code Searcher for backend, resolving tokens and
// instance URLs from cfg exactly as the `ketch code` CLI does. Both the CLI
// and the MCP server call this — it is the single owner of the backend switch.
func NewFromConfig(cfg *config.Config, backend string) (Searcher, error) {
	switch backend {
	case "sourcegraph":
		return NewSourcegraph(cfg.SourcegraphURL), nil
	case "grepapp":
		return NewGrepApp(), nil
	case "github":
		token, _ := cfg.ResolveGithubToken()
		if token == "" {
			return nil, fmt.Errorf(`github code search: no token found.
  - explicit:   ketch config set github_token <token>
  - env var:    export GITHUB_TOKEN=<token>
  - or run:     gh auth login`)
		}
		return NewGitHub(token), nil
	case "firecrawl":
		// Unlike search's firecrawl backend, this one always requires a key:
		// the Developer Index is hosted-only, with no self-hosted equivalent
		// that a keyless firecrawl_url could reach.
		keys := cfg.FirecrawlKeys()
		if len(keys) == 0 {
			return nil, fmt.Errorf(`firecrawl code search: no API key found.
  - explicit:   ketch config set firecrawl_api_key <key>
  - env var:    export KETCH_FIRECRAWL_API_KEY=<key>
  - free key:   https://firecrawl.dev`)
		}
		return NewFirecrawl(keys, cfg.EffectiveFirecrawlURL()), nil
	default:
		return nil, fmt.Errorf("%w %q (available: %s)", ErrUnknownBackend, backend, strings.Join(config.AvailableCodeBackends(), ", "))
	}
}
