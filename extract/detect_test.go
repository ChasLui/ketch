package extract

import (
	"os"
	"strings"
	"testing"
)

func TestDetectJSShell(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "minimal real content is static",
			html: `
				<!doctype html>
				<html>
					<head><title>Article</title></head>
					<body>
						<main>
							<article>
								<h1>Shipping Notes</h1>
								<p>This page has actual content for extraction and is meant to look like a conventional server-rendered article rather than a JavaScript bootstrap shell with placeholders.</p>
								<p>The second paragraph adds enough visible text to exceed the threshold, which should cause the detector to classify the document as static even if the markup is otherwise minimal.</p>
							</article>
						</main>
					</body>
				</html>
			`,
			want: "static",
		},
		{
			name: "salesforce shell is likely shell",
			html: `
				<!doctype html>
				<html>
					<head>
						<title>Lightning</title>
						<script>window.__BOOTSTRAP__ = {"routes":["a","b","c"],"payload":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"};</script>
						<script src="/assets/app.js"></script>
					</head>
					<body>
						<div id="app">Loading</div>
						<noscript>This app requires JavaScript and redirects when JavaScript is disabled.</noscript>
					</body>
				</html>
			`,
			want: "likely_shell",
		},
		{
			name: "react spa shell is likely shell",
			html: `
				<!doctype html>
				<html>
					<head>
						<title>App</title>
						<script id="__NEXT_DATA__" type="application/json">
							{"buildId":"dev","page":"/","props":{"chunks":["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]}}
						</script>
					</head>
					<body>
						<div id="root"></div>
					</body>
				</html>
			`,
			want: "likely_shell",
		},
		{
			name: "short real page is ambiguous",
			html: `
				<!doctype html>
				<html>
					<head><title>Not Found</title></head>
					<body>
						<main>
							<h1>Not Found</h1>
							<p>The page you requested could not be located.</p>
						</main>
					</body>
				</html>
			`,
			want: "ambiguous",
		},
		{
			name: "js loading page with fallback description is likely shell",
			html: `
				<!doctype html>
				<html>
					<head><title>Flowchart Maker</title></head>
					<body>
						<div id="geInfo">
							<h1>Flowchart Maker and Online Diagram Software</h1>
							<p>draw.io is free online diagram software. You can use it as a flowchart maker, network diagram software, to create UML online, as an ER diagram tool, to design database schema, and more.</p>
							<h2>Loading... <img src="spin.gif"/></h2>
							<p>Please ensure JavaScript is enabled.</p>
						</div>
						<script src="js/main.js"></script>
					</body>
				</html>
			`,
			want: "likely_shell",
		},
		{
			name: "ssr next page with real content is static",
			html: `
				<!doctype html>
				<html>
					<head>
						<title>SSR Next</title>
						<script id="__NEXT_DATA__" type="application/json">
							{"page":"/docs","props":{"pageProps":{"title":"SSR"}}}
						</script>
					</head>
					<body>
						<div id="__next">
							<main>
								<article>
									<h1>Rendered Content</h1>
									<p>The initial HTML already includes the full article body, so the detector should treat the document as static even though the page carries the standard Next.js bootstrap data.</p>
									<p>This extra paragraph ensures there is comfortably more than two hundred characters of visible text in the extraction selectors and prevents a false positive.</p>
								</article>
							</main>
						</div>
					</body>
				</html>
			`,
			want: "static",
		},
		{
			name: "next.js app router shell with thin content is likely shell",
			html: `
				<!doctype html>
				<html>
					<head><title>Pricing</title></head>
					<body>
						<div id="root"></div>
						<script>self.__next_f=self.__next_f||[];self.__next_f.push([1,"pricing rows streamed via rsc"]);</script>
					</body>
				</html>
			`,
			want: "likely_shell",
		},
		{
			name: "react 18 streaming hydration container is likely shell",
			html: `
				<!doctype html>
				<html>
					<head><title>App</title></head>
					<body>
						<div id="_r_"></div>
						<!--$--><!--/$-->
					</body>
				</html>
			`,
			want: "likely_shell",
		},
		{
			name: "vue 3 data-v-app shell is likely shell",
			html: `
				<!doctype html>
				<html>
					<head><title>Dashboard</title></head>
					<body>
						<div data-v-app=""></div>
					</body>
				</html>
			`,
			want: "likely_shell",
		},
		{
			// Regression for the >=200-char Phase-1 short-circuit: server-rendered
			// chrome carries plenty of visible text, but the real content streams
			// in client-side (RSC payload dwarfs the visible text). Must escalate.
			name: "ssr chrome with client-rendered body overrides short-circuit",
			html: appRouterShellChromeHTML(),
			want: "likely_shell",
		},
		{
			// Guard against false positives: a genuinely server-rendered App
			// Router page (full prose in the initial HTML, proportional RSC
			// payload) must stay static despite carrying __next_f.
			name: "ssr app router with real content stays static",
			html: appRouterSSRHTML(),
			want: "static",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := DetectJSShell(tt.html); got != tt.want {
				t.Fatalf("DetectJSShell() = %q, want %q", got, tt.want)
			}
		})
	}
}

// appRouterShellChromeHTML builds a page that mimics the bitdeer pricing shell:
// real server-rendered chrome (>200 chars of nav/marketing text) but the actual
// content streams in via a Next.js App Router RSC payload that dwarfs the
// visible text. Exercises the Phase-1 short-circuit override.
func appRouterShellChromeHTML() string {
	chrome := `<p>Bitdeer AI Cloud delivers scalable GPU infrastructure for training and inference across global regions, with transparent per-token pricing for every model.</p>
		<p>Sign in to the console to launch dedicated endpoints, monitor usage, and manage billing for your organization across all supported regions.</p>`
	// RSC payload comfortably exceeding 8x the visible chrome (which scanVisible
	// counts on both the <main> and its nested <p> elements).
	payload := strings.Repeat(`self.__next_f.push([1,"a:[\"$\",\"tr\",null,{\"children\":\"price table row streamed via rsc\"}]"]);`, 120)
	return `<!doctype html><html><head><title>Pricing</title></head><body>` +
		`<nav><ul><li>Products</li><li>Pricing</li><li>Docs</li></ul></nav>` +
		`<main>` + chrome + `</main><div id="__next"></div>` +
		`<script>` + payload + `</script></body></html>`
}

// appRouterSSRHTML builds a genuinely server-rendered App Router page: the full
// prose is in the initial HTML and the RSC payload is proportional (well under
// the 8x override threshold), so it must stay static despite carrying __next_f.
func appRouterSSRHTML() string {
	body := strings.Repeat(`<p>The Next.js App Router renders this article on the server, so the full prose is present in the initial HTML and the detector should keep treating it as static content.</p>`, 6)
	payload := `self.__next_f.push([1,"the next.js app router renders this article on the server"]);`
	return `<!doctype html><html><head><title>Guide</title></head><body>` +
		`<main><article><h1>App Router Guide</h1>` + body + `</article></main>` +
		`<script>` + payload + `</script></body></html>`
}

// TestDetectBitdeerPricingFixtureIsShell pins the real-world regression from
// issue #15: the bitdeer pricing page is a Next.js App Router shell (content via
// RSC) and must be detected as needing browser rendering.
func TestDetectBitdeerPricingFixtureIsShell(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/bitdeer-pricing.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if got := DetectJSShell(string(body)); got != "likely_shell" {
		t.Fatalf("DetectJSShell(bitdeer) = %q, want %q", got, "likely_shell")
	}
}

// TestDetectorExtraMarkersEscalate verifies operator-configured spa_markers are
// honored on both the low-text corroborator path and the high-text override.
func TestDetectorExtraMarkersEscalate(t *testing.T) {
	t.Parallel()

	// Low-text page whose only signal is a custom marker.
	lowText := `<!doctype html><html><head><title>X</title></head><body><div id="mount"></div></body></html>`
	if got := DetectJSShell(lowText); got == "likely_shell" {
		t.Fatalf("baseline DetectJSShell = %q, expected not likely_shell without marker", got)
	}
	d := NewDetector([]string{"id=\"mount\""})
	if got := d.DetectJSShell(lowText); got != "likely_shell" {
		t.Fatalf("with extra marker DetectJSShell = %q, want likely_shell", got)
	}

	// Blank markers must be dropped so they never match every page.
	dBlank := NewDetector([]string{"  ", ""})
	if got := dBlank.DetectJSShell(lowText); got == "likely_shell" {
		t.Fatalf("blank markers should be ignored, got %q", got)
	}
}
