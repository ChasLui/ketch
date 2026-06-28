package extract

import (
	"os"
	"strings"
	"testing"
)

func TestExtractReadabilityRendersCleanTheadTable(t *testing.T) {
	t.Parallel()

	html := `<!doctype html>
<html>
<head><title>City Ages</title></head>
<body>
	<nav>Ignored navigation</nav>
	<main>
		<article>
			<h1>City Ages</h1>
			<p>This article has enough prose for readability to keep the main content and the table below describes the sample data.</p>
			<table>
				<thead>
					<tr><th>Name</th><th>City</th><th>Age</th></tr>
				</thead>
				<tbody>
					<tr><td>Max</td><td>Berlin</td><td>20</td></tr>
					<tr><td>Ada</td><td>London</td><td>37</td></tr>
				</tbody>
			</table>
		</article>
	</main>
</body>
</html>`

	result, err := New().Extract("https://example.test/cities", html)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	assertContainsAll(t, result.Markdown,
		"|Name|City|Age|",
		"|---|---|---|",
		"|Max|Berlin|20|",
		"|Ada|London|37|",
	)
}

func TestExtractRawPromotesHeaderForNoTheadTable(t *testing.T) {
	t.Parallel()

	result, err := extractRaw(`<!doctype html>
<html>
<head><title>Pricing</title></head>
<body>
	<main>
		<table>
			<tbody>
				<tr><td>Plan</td><td>Price</td></tr>
				<tr><td>Free</td><td>$0</td></tr>
				<tr><td>Pro</td><td>$10</td></tr>
			</tbody>
		</table>
	</main>
</body>
</html>`)
	if err != nil {
		t.Fatalf("extractRaw: %v", err)
	}
	assertContainsAll(t, result.Markdown,
		"|Plan|Price|",
		"|---|---|",
		"|Free|$0|",
		"|Pro|$10|",
	)
}

func TestExtractSelectorRendersColspanRowspanWithEmptyCells(t *testing.T) {
	t.Parallel()

	markdown, err := ExtractSelector(`<!doctype html>
<html><body>
	<table id="plans">
		<thead><tr><th>Tier</th><th>Input</th><th>Output</th></tr></thead>
		<tbody>
			<tr><td rowspan="2">Pro</td><td colspan="2">Included</td></tr>
			<tr><td>$1</td><td>$2</td></tr>
		</tbody>
	</table>
</body></html>`, "#plans")
	if err != nil {
		t.Fatalf("ExtractSelector: %v", err)
	}
	assertContainsAll(t, markdown,
		"|Tier|Input|Output|",
		"|---|---|---|",
		"|Pro|Included||",
		"||$1|$2|",
	)
	if strings.Contains(markdown, "Included|Included") {
		t.Fatalf("colspan cells must be empty, not mirrored:\n%s", markdown)
	}
}

func TestExtractSelectorPreservesMultilineCellTable(t *testing.T) {
	t.Parallel()

	markdown, err := ExtractSelector(`<!doctype html>
<html><body>
	<table id="features">
		<tbody>
			<tr><td>Feature</td><td>Details</td></tr>
			<tr><td>Support</td><td>Line one<br>Line two</td></tr>
		</tbody>
	</table>
</body></html>`, "#features")
	if err != nil {
		t.Fatalf("ExtractSelector: %v", err)
	}
	assertContainsAll(t, markdown,
		"|Feature|Details|",
		"|---|---|",
		"|Support|Line one  <br />Line two|",
	)
}

func TestExtractSelectorSkipsPresentationLayoutTable(t *testing.T) {
	t.Parallel()

	markdown, err := ExtractSelector(`<!doctype html>
<html><body>
	<table id="layout" role="presentation">
		<tr><td>Nav</td><td>Layout</td></tr>
		<tr><td>A</td><td>B</td></tr>
	</table>
</body></html>`, "#layout")
	if err != nil {
		t.Fatalf("ExtractSelector: %v", err)
	}
	if strings.Contains(markdown, "|---|") || strings.Contains(markdown, "|Nav|Layout|") {
		t.Fatalf("presentation table rendered as a pipe table:\n%s", markdown)
	}
	assertContainsAll(t, markdown, "Nav", "Layout", "A", "B")
}

func TestTokenReplyPricingFixtureHasNoServerRenderedTable(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/tokenreply-pricing.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	lower := strings.ToLower(string(body))
	assertContainsAll(t, lower, "tokenreply", "pricing")
	if strings.Contains(lower, "<table") {
		t.Fatalf("fixture unexpectedly contains a server-rendered table")
	}
}

func TestExtractFallsBackToRawWhenReadabilityDropsTable(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("testdata/tokenreply-pricing.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	withTable := strings.Replace(
		string(body),
		`<div id="root"></div>`,
		`<div id="root"><table><tr><td>Plan</td><td>Price</td></tr><tr><td>Pro</td><td>$10</td></tr></table></div>`,
		1,
	)
	result, err := New().Extract("https://www.tokenreply.com/pricing", withTable)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if strings.TrimSpace(result.Markdown) == "" {
		t.Fatalf("expected raw-table fallback, got empty markdown")
	}
	assertContainsAll(t, result.Markdown, "|Plan|Price|", "|---|---|", "|Pro|$10|")
}

func assertContainsAll(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
