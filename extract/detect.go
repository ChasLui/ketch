package extract

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Detector classifies raw HTML as "static" (has content), "likely_shell"
// (needs browser rendering), or "ambiguous". The zero value is ready to use
// and matches only the built-in framework markers; NewDetector folds in
// operator-configured markers (config spa_markers) on top of the built-ins.
type Detector struct {
	extraMarkers []string // operator-supplied markers, pre-lowercased
}

// NewDetector returns a Detector that also matches the given operator-supplied
// SPA markers (substrings of the lowercased HTML) in addition to the built-in
// ones. Markers are lowercased and trimmed; blank entries are dropped so a
// stray "" can never match every page.
func NewDetector(extraMarkers []string) *Detector {
	var ms []string
	for _, m := range extraMarkers {
		if m = strings.ToLower(strings.TrimSpace(m)); m != "" {
			ms = append(ms, m)
		}
	}
	return &Detector{extraMarkers: ms}
}

// defaultDetector backs the package-level functions (built-in markers only).
var defaultDetector = &Detector{}

// DetectJSShell analyzes raw HTML and returns whether the page appears to be
// a JavaScript shell that needs browser rendering for content extraction.
// Returns: "static" (has content), "likely_shell" (needs browser), or "ambiguous".
//
// It uses only the built-in markers. Callers with operator-configured markers
// should build a Detector via NewDetector and call its method instead.
func DetectJSShell(rawHTML string) string {
	return defaultDetector.DetectJSShell(rawHTML)
}

// DetectJSShellFromDoc is the core detector given an already-parsed document
// and its raw source. Useful when callers have already paid for the parse.
// It uses only the built-in markers; see NewDetector for operator markers.
func DetectJSShellFromDoc(doc *goquery.Document, rawHTML string) string {
	return defaultDetector.DetectJSShellFromDoc(doc, rawHTML)
}

// DetectJSShell parses rawHTML and classifies it. See DetectJSShell.
func (d *Detector) DetectJSShell(rawHTML string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return "ambiguous"
	}
	return d.DetectJSShellFromDoc(doc, rawHTML)
}

// DetectJSShellFromDoc classifies an already-parsed document. See
// DetectJSShellFromDoc.
func (d *Detector) DetectJSShellFromDoc(doc *goquery.Document, rawHTML string) string {
	// Phase 1: collect visible text + meaningful block count in a single pass.
	// Most pages short-circuit here without touching script/noscript/body.
	vis := scanVisible(doc)

	if len(vis.text) >= 200 {
		// Pages that explicitly require JS AND say "loading" are shells,
		// even when they include a fallback description (e.g. draw.io).
		// isJSLoadingPage handles the lowercase internally and only pays
		// that cost on pages whose visible text mentions "loading".
		if isJSLoadingPage(vis.text) {
			return "likely_shell"
		}
		// Content-is-client-rendered override. Server-rendered chrome (nav,
		// footer, marketing copy) can carry >200 chars of real text while the
		// actual content streams in client-side — e.g. Next.js App Router
		// pages whose body lives in the RSC payload (self.__next_f.push), or
		// Vue 3 / SvelteKit / Qwik / Astro islands hydrating an empty mount.
		// Escalate only when BOTH a strong hydration/streaming marker is
		// present AND the script payload dwarfs the visible text: each gate
		// alone is too noisy, and every escalation costs a browser render, so
		// the conjunction keeps false positives (and Chrome renders) bounded.
		// Cheap gate first — clientRenderedPayload avoids the full-source
		// lowercase that hasClientRenderMarker pays.
		if clientRenderedPayload(doc, vis.text) && d.hasClientRenderMarker(rawHTML) {
			return "likely_shell"
		}
		return "static"
	}

	if vis.meaningfulBlocks > 2 {
		return "ambiguous"
	}

	// Phase 2: low-text pages need corroborators. Cheap DOM-local checks
	// run first; the full-source lowercase is only paid for if they miss.
	if d.hasCorroborator(doc, vis, rawHTML) {
		return "likely_shell"
	}
	return "ambiguous"
}

type visibleStats struct {
	text             string
	meaningfulBlocks int
}

// scanVisible traverses the content selectors once. It's the only work we
// do on the hot "static" path.
func scanVisible(doc *goquery.Document) visibleStats {
	var s visibleStats
	visible := make([]string, 0, 16)
	doc.Find("p, article, main, section, h1, h2, h3, h4, h5, h6, li, td, th, dd, dt, blockquote").
		Each(func(_ int, sel *goquery.Selection) {
			text := normalizeWhitespace(sel.Text())
			if text == "" {
				return
			}
			visible = append(visible, text)
			switch goquery.NodeName(sel) {
			case "p", "li", "h1", "h2", "h3", "h4", "h5", "h6", "td", "blockquote", "dd":
				if len(text) > 20 {
					s.meaningfulBlocks++
				}
			}
		})
	s.text = strings.Join(visible, " ")
	return s
}

// hasCorroborator runs the expensive signals — script bytes, noscript text,
// body JS messages, and string markers against the HTML. Cheaper DOM-local
// checks run first, and the full-source lowercase is deferred until the
// cheap checks miss.
func (d *Detector) hasCorroborator(doc *goquery.Document, vis visibleStats, rawHTML string) bool {
	if noscriptMentionsJS(doc) {
		return true
	}
	if bodyRequiresJavaScript(doc) {
		return true
	}
	lowerHTML := strings.ToLower(rawHTML)
	if d.hasSPAShellMarker(lowerHTML) || hasLowTextAppShellMarker(lowerHTML) {
		return true
	}
	return highScriptToTextRatio(doc, vis.text)
}

func noscriptMentionsJS(doc *goquery.Document) bool {
	found := false
	doc.Find("noscript").EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		if strings.Contains(strings.ToLower(sel.Text()), "javascript") {
			found = true
			return false
		}
		return true
	})
	return found
}

func bodyRequiresJavaScript(doc *goquery.Document) bool {
	return requiresJavaScript(strings.ToLower(doc.Find("body").Text()))
}

// sumScriptBytes totals the text length of every <script> element, including
// inline JSON / RSC payloads (e.g. <script>self.__next_f.push(...)</script>
// and <script type="application/json">). This is the script side of the
// script-to-text ratio used by both the low-text corroborator and the
// high-text client-render override.
func sumScriptBytes(doc *goquery.Document) int {
	n := 0
	doc.Find("script").Each(func(_ int, sel *goquery.Selection) {
		n += len(sel.Text())
	})
	return n
}

// highScriptToTextRatio is the low-text corroborator gate: script bytes exceed
// visible text by 3×.
func highScriptToTextRatio(doc *goquery.Document, visibleText string) bool {
	return scriptExceedsText(sumScriptBytes(doc), len(visibleText), 3)
}

// clientRenderedPayload is the high-text override gate: the script/JSON payload
// must dwarf the visible text by a wide margin (8×, deliberately stricter than
// the low-text corroborator's 3×). Most pages with >200 chars of visible text
// are genuinely static, so a higher bar is needed before we trust a hydration
// marker enough to pay for a browser render.
func clientRenderedPayload(doc *goquery.Document, visibleText string) bool {
	return scriptExceedsText(sumScriptBytes(doc), len(visibleText), 8)
}

func scriptExceedsText(scriptLen, visibleLen, mult int) bool {
	if visibleLen == 0 {
		visibleLen = 1
	}
	return scriptLen > visibleLen*mult
}

// isJSLoadingPage detects pages that have fallback content but are actually
// JS loading screens (e.g. draw.io's splash with marketing blurb). Requires
// BOTH a loading indicator AND an explicit JS requirement. Lowercases
// lazily — only when the first check passes.
func isJSLoadingPage(visibleText string) bool {
	lower := strings.ToLower(visibleText)
	return strings.Contains(lower, "loading") && requiresJavaScript(lower)
}

func requiresJavaScript(lower string) bool {
	return strings.Contains(lower, "enable javascript") ||
		strings.Contains(lower, "requires javascript") ||
		strings.Contains(lower, "ensure javascript") ||
		strings.Contains(lower, "javascript is required") ||
		strings.Contains(lower, "javascript is disabled")
}

// spaMarkers are substring signals (matched in lowercased HTML) that a
// low-text page is a framework shell needing browser rendering. All markers
// are ASCII; the input is pre-lowercased.
var spaMarkers = []string{
	// Legacy framework roots.
	`id="__next"`, `id='__next'`,
	`id="__nuxt"`, `id='__nuxt'`,
	`data-reactroot`,
	`ng-version=`,
	`<app-root`,
	`id="___gatsby"`, `id='___gatsby'`,
	`__next_data__`,
	`__nuxt__`,
	// Modern hydration / streaming signals.
	`__next_f`,             // Next.js App Router RSC streaming (self.__next_f.push)
	`id="_r_"`, `id='_r_'`, // React 18 streaming hydration container
	`data-v-app`,   // Vue 3 (createApp().mount)
	`data-svelte`,  // Svelte components
	`__sveltekit`,  // SvelteKit hydration globals
	`q:container`,  // Qwik
	`astro-island`, // Astro islands
}

// clientRenderMarkers are the strong subset signalling that the page *content*
// (not merely the chrome) is rendered client-side. Used by the high-text
// "content is client-rendered" override, which additionally requires a
// dominant script payload. Deliberately excludes legacy roots (id="__next",
// data-reactroot, …) that are also present on fully server-rendered pages and
// would over-trigger the override.
var clientRenderMarkers = []string{
	`__next_f`,                                       // Next.js App Router RSC streaming
	`data-v-app`,                                     // Vue 3
	`__sveltekit`,                                    // SvelteKit
	`data-svelte`,                                    // Svelte
	`q:container`,                                    // Qwik
	`astro-island`,                                   // Astro islands
	`<!--$-->`,                                       // React 18 streaming suspense boundary
	`<div id="root"></div>`, `<div id='root'></div>`, // empty React mount node
	`<div id="app"></div>`, `<div id='app'></div>`, // empty Vue/generic mount node
}

func (d *Detector) hasSPAShellMarker(lowerHTML string) bool {
	for _, marker := range spaMarkers {
		if strings.Contains(lowerHTML, marker) {
			return true
		}
	}
	for _, marker := range d.extraMarkers {
		if strings.Contains(lowerHTML, marker) {
			return true
		}
	}
	return false
}

// hasClientRenderMarker reports whether a strong client-render marker (or an
// operator-configured marker) is present. Lowercases the full source, so it is
// only called once the cheaper clientRenderedPayload gate has passed.
func (d *Detector) hasClientRenderMarker(rawHTML string) bool {
	lower := strings.ToLower(rawHTML)
	for _, marker := range clientRenderMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, marker := range d.extraMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func hasLowTextAppShellMarker(lowerHTML string) bool {
	return strings.Contains(lowerHTML, `id="app"`) || strings.Contains(lowerHTML, `id='app'`)
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
