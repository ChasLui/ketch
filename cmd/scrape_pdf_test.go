package cmd

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/1broseidon/ketch/extract"
	"github.com/1broseidon/ketch/scrape"
)

func TestCLIScrapePDF(t *testing.T) {
	withDefaultConfig(t)
	server := pdfTestServer(t)
	output, err := executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--no-llms-txt", server.URL + "/document.pdf"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(output, "Ketch PDF extraction works.") {
		t.Fatalf("output = %q", output)
	}
}

func TestCLIScrapePDFRejectsSelector(t *testing.T) {
	withDefaultConfig(t)
	server := pdfTestServer(t)
	_, err := executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--no-llms-txt", "--select", "main", server.URL + "/document.pdf"})
	assertPDFValidationError(t, err, scrape.ErrPDFSelectorUnsupported)
}

func TestCLIScrapePDFRejectsRaw(t *testing.T) {
	withDefaultConfig(t)
	server := pdfTestServer(t)
	_, err := executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--raw", server.URL + "/document.pdf"})
	assertPDFValidationError(t, err, scrape.ErrPDFRawUnsupported)
}

func TestCLIScrapePDFForceBrowserClassifiesBeforeBrowserPrecondition(t *testing.T) {
	withDefaultConfig(t)
	server := pdfTestServer(t)
	url := server.URL + "/document.pdf"

	output, err := executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--no-llms-txt", "--force-browser", url})
	if err != nil {
		t.Fatalf("forced markdown scrape: %v", err)
	}
	if !strings.Contains(output, "Ketch PDF extraction works.") {
		t.Fatalf("output = %q", output)
	}

	_, err = executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--force-browser", "--raw", url})
	assertPDFValidationError(t, err, scrape.ErrPDFRawUnsupported)
	_, err = executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--no-llms-txt", "--force-browser", "--select", "main", url})
	assertPDFValidationError(t, err, scrape.ErrPDFSelectorUnsupported)
}

func TestCLIScrapePDFNoTextIsPrecondition(t *testing.T) {
	withDefaultConfig(t)
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

	_, err = executeScrapeCaptureStdout(t, []string{"scrape", "--no-cache", "--no-llms-txt", server.URL + "/scanned.pdf"})
	if err == nil {
		t.Fatal("expected precondition error")
	}
	var exitError *ExitError
	if !errors.As(err, &exitError) || exitError.Code != ExitPrecondition {
		t.Fatalf("error = %v, want exit %d", err, ExitPrecondition)
	}
	if !errors.Is(err, extract.ErrPDFNoText) {
		t.Fatalf("error = %v, want ErrPDFNoText", err)
	}
	if !strings.Contains(err.Error(), "OCR-capable converter") {
		t.Fatalf("error = %v, want OCR hint", err)
	}
}

func pdfTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	pdf, err := os.ReadFile("../extract/testdata/simple.pdf")
	if err != nil {
		t.Fatalf("read PDF fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(server.Close)
	return server
}

func executeScrapeCaptureStdout(t *testing.T, args []string) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()

	command := buildScrapeCmd(args)
	execErr := command.Execute()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(output), execErr
}

func assertPDFValidationError(t *testing.T, err, sentinel error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error")
	}
	var exitError *ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("error type = %T, want *ExitError: %v", err, err)
	}
	if exitError.Code != ExitValidation {
		t.Fatalf("exit code = %d, want %d", exitError.Code, ExitValidation)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
}
