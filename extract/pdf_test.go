package extract

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestBuiltInPDFExtractorFixture(t *testing.T) {
	src := readPDFFixture(t)

	markdown, err := NewPDFExtractor().Extract(t.Context(), src)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(markdown, "Ketch PDF extraction works.") {
		t.Fatalf("extracted text = %q", markdown)
	}
}

func TestBuiltInPDFExtractorMalformed(t *testing.T) {
	_, err := NewPDFExtractor().Extract(t.Context(), []byte("not a PDF"))
	if err == nil {
		t.Fatal("expected malformed PDF error")
	}
}

func TestBuiltInPDFExtractorNoText(t *testing.T) {
	src := readPDFFixture(t)
	phrase := []byte("Ketch PDF extraction works.")
	src = bytes.Replace(src, phrase, bytes.Repeat([]byte(" "), len(phrase)), 1)

	_, err := NewPDFExtractor().Extract(t.Context(), src)
	if !errors.Is(err, ErrPDFNoText) {
		t.Fatalf("error = %v, want ErrPDFNoText", err)
	}
}

func TestBuiltInPDFExtractorCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewPDFExtractor().Extract(ctx, readPDFFixture(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func readPDFFixture(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/simple.pdf")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return src
}
