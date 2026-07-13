package mcp

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/1broseidon/ketch/extract"
	"github.com/1broseidon/ketch/scrape"
)

func TestMCPScrapePDFNoTextIsPrecondition(t *testing.T) {
	pdf, err := os.ReadFile("../extract/testdata/simple.pdf")
	if err != nil {
		t.Fatalf("read PDF fixture: %v", err)
	}
	phrase := []byte("Ketch PDF extraction works.")
	pdf = bytes.Replace(pdf, phrase, bytes.Repeat([]byte(" "), len(phrase)), 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)

	s := &Server{scraper: scrape.New()}
	_, err = s.scrapeOne(t.Context(), server.URL+"/scanned.pdf", ScrapeInput{NoCache: true, NoLLMSTxt: true})
	if err == nil || !strings.HasPrefix(err.Error(), "[precondition] ") {
		t.Fatalf("error = %v, want [precondition] prefix", err)
	}
	if !errors.Is(err, extract.ErrPDFNoText) {
		t.Fatalf("error = %v, want ErrPDFNoText", err)
	}
	if !strings.Contains(err.Error(), "OCR-capable converter") {
		t.Fatalf("error = %v, want OCR hint", err)
	}
}

func TestMCPScrapeForceBrowserClassifiesPDFBeforeBrowserPrecondition(t *testing.T) {
	pdf, err := os.ReadFile("../extract/testdata/simple.pdf")
	if err != nil {
		t.Fatalf("read PDF fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)

	s := &Server{scraper: scrape.New()}
	url := server.URL + "/document.pdf"
	markdownInput := ScrapeInput{URL: url, ForceBrowser: true, NoCache: true, NoLLMSTxt: true}
	if err := s.validateScrapeInput(markdownInput); err != nil {
		t.Fatalf("validate forced PDF input: %v", err)
	}
	result, err := s.scrapeOne(t.Context(), url, markdownInput)
	if err != nil {
		t.Fatalf("forced markdown scrape: %v", err)
	}
	if !strings.Contains(result.Markdown, "Ketch PDF extraction works.") {
		t.Fatalf("markdown = %q", result.Markdown)
	}

	for _, input := range []ScrapeInput{
		{Raw: true, ForceBrowser: true, NoCache: true},
		{Selector: "main", ForceBrowser: true, NoCache: true},
	} {
		_, err := s.scrapeOne(t.Context(), url, input)
		if err == nil || !strings.HasPrefix(err.Error(), "[validation] ") {
			t.Fatalf("error = %v, want [validation] prefix", err)
		}
	}
}
