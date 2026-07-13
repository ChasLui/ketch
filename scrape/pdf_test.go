package scrape

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/1broseidon/ketch/config"
)

func TestScrapePDFByMIMEType(t *testing.T) {
	pdf := readPDFTestFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "application/pdf") {
			t.Errorf("Accept header = %q, want application/pdf", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)

	s := New()
	page, source, err := s.Scrape(context.Background(), server.URL+"/document")
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if source != SourceHTTP {
		t.Fatalf("source = %q, want %q", source, SourceHTTP)
	}
	if !strings.Contains(page.Markdown, "Ketch PDF extraction works.") {
		t.Fatalf("markdown = %q", page.Markdown)
	}
}

func TestScrapePDFMagicByteFallback(t *testing.T) {
	pdf := readPDFTestFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)

	result, err := New().ScrapeConditional(t.Context(), server.URL+"/download", "", "")
	if err != nil {
		t.Fatalf("ScrapeConditional: %v", err)
	}
	if result.ContentType != "application/pdf" {
		t.Fatalf("ContentType = %q, want application/pdf", result.ContentType)
	}
	if !strings.Contains(result.Page.Markdown, "Ketch PDF extraction works.") {
		t.Fatalf("markdown = %q", result.Page.Markdown)
	}
	if result.Doc != nil || result.JSDetection != "" || result.RawHTML != "" {
		t.Fatalf("PDF entered HTML pipeline: %#v", result)
	}
}

func TestForceBrowserClassifiesPDFBeforeRendering(t *testing.T) {
	server := pdfFixtureServer(t)
	browser := &pdfViewerBrowser{}
	s := NewWithBrowserConn(browser, nil)

	page, err := s.ScrapeMarkdown(t.Context(), nil, server.URL+"/document.pdf", true)
	if err != nil {
		t.Fatalf("ScrapeMarkdown: %v", err)
	}
	if !strings.Contains(page.Markdown, "Ketch PDF extraction works.") {
		t.Fatalf("markdown = %q", page.Markdown)
	}
	if browser.calls != 0 {
		t.Fatalf("browser calls = %d, want 0", browser.calls)
	}
}

func TestForceBrowserRejectsPDFHTMLModesBeforeRendering(t *testing.T) {
	server := pdfFixtureServer(t)

	for _, test := range []struct {
		name string
		run  func(*Scraper) error
		want error
	}{
		{
			name: "raw",
			run: func(s *Scraper) error {
				_, _, _, err := s.ScrapeRaw(t.Context(), nil, server.URL+"/document.pdf", true)
				return err
			},
			want: ErrPDFRawUnsupported,
		},
		{
			name: "selector",
			run: func(s *Scraper) error {
				_, err := s.ScrapeSelector(t.Context(), server.URL+"/document.pdf", "main", true)
				return err
			},
			want: ErrPDFSelectorUnsupported,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			browser := &pdfViewerBrowser{}
			s := NewWithBrowserConn(browser, nil)
			err := test.run(s)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if browser.calls != 0 {
				t.Fatalf("browser calls = %d, want 0", browser.calls)
			}
		})
	}
}

func TestConfiguredExternalPDFExtractorIsAuthoritative(t *testing.T) {
	pdf := readPDFTestFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)

	cfg := config.Defaults()
	cfg.ExternalPDFToMDConverterCommand = "ketch-test-converter-that-does-not-exist {input}"
	s, err := NewFromConfig(&cfg)
	if err != nil {
		t.Fatalf("NewFromConfig: %v", err)
	}
	_, _, err = s.Scrape(t.Context(), server.URL+"/document.pdf")
	if err == nil || !strings.Contains(err.Error(), "external PDF converter failed") {
		t.Fatalf("error = %v; built-in extractor must not be used as fallback", err)
	}
}

func TestEffectiveContentTypeHonorsPDFMIME(t *testing.T) {
	if got := effectiveContentType("application/pdf; version=1.7", []byte("not magic")); got != "application/pdf" {
		t.Fatalf("effectiveContentType = %q, want application/pdf", got)
	}
}

func TestScrapeHTMLUnchanged(t *testing.T) {
	html := `<!doctype html><html><head><title>HTML page</title></head><body><main><h1>Still HTML</h1><p>Ordinary extraction remains unchanged.</p></main></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(html))
	}))
	t.Cleanup(server.Close)

	result, err := New().ScrapeConditional(t.Context(), server.URL, "", "")
	if err != nil {
		t.Fatalf("ScrapeConditional: %v", err)
	}
	if result.ContentType != "text/html" {
		t.Fatalf("ContentType = %q, want text/html", result.ContentType)
	}
	if result.Page.Title != "HTML page" || !strings.Contains(result.Page.Markdown, "Still HTML") {
		t.Fatalf("page = %#v", result.Page)
	}
	if result.RawHTML != html {
		t.Fatalf("RawHTML changed: %q", result.RawHTML)
	}
}

type pdfViewerBrowser struct {
	calls int
}

func (b *pdfViewerBrowser) Fetch(context.Context, string) (string, error) {
	b.calls++
	return `<html><body>Chromium PDF viewer</body></html>`, nil
}

func (b *pdfViewerBrowser) Close() {}

func pdfFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	pdf := readPDFTestFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)
	return server
}

func readPDFTestFixture(t *testing.T) []byte {
	t.Helper()
	pdf, err := os.ReadFile("../extract/testdata/simple.pdf")
	if err != nil {
		t.Fatalf("read PDF fixture: %v", err)
	}
	return pdf
}
