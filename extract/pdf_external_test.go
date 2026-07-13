package extract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

type fakePDFCommandRunner struct {
	run func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error
}

func (f fakePDFCommandRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	return f.run(ctx, name, args, stdout, stderr)
}

func TestExternalPDFExtractorReplacesInputPlaceholder(t *testing.T) {
	var inputPath string
	runner := fakePDFCommandRunner{run: func(_ context.Context, name string, args []string, stdout, _ io.Writer) error {
		if name != "converter" {
			t.Fatalf("name = %q, want converter", name)
		}
		if len(args) != 3 || !strings.HasPrefix(args[0], "--source=") || args[1] != "--format" || args[2] != "md" {
			t.Fatalf("args = %#v", args)
		}
		inputPath = strings.TrimPrefix(args[0], "--source=")
		got, err := os.ReadFile(inputPath)
		if err != nil {
			t.Fatalf("read temporary input: %v", err)
		}
		if string(got) != "pdf bytes" {
			t.Fatalf("temporary input = %q", got)
		}
		_, _ = io.WriteString(stdout, "# Converted\n")
		return nil
	}}
	extractor, err := newExternalPDFExtractor(`converter --source="{input}" --format md`, time.Second, runner)
	if err != nil {
		t.Fatalf("newExternalPDFExtractor: %v", err)
	}

	markdown, err := extractor.Extract(t.Context(), []byte("pdf bytes"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if markdown != "# Converted" {
		t.Fatalf("markdown = %q", markdown)
	}
	if inputPath == "" {
		t.Fatal("runner did not receive an input path")
	}
}

func TestExternalPDFExtractorCleansUpTemporaryInput(t *testing.T) {
	var inputPath string
	runner := fakePDFCommandRunner{run: func(_ context.Context, _ string, args []string, stdout, _ io.Writer) error {
		inputPath = args[0]
		if _, err := os.Stat(inputPath); err != nil {
			t.Fatalf("temporary input missing during run: %v", err)
		}
		_, _ = io.WriteString(stdout, "converted")
		return nil
	}}
	extractor, err := newExternalPDFExtractor("converter {input}", time.Second, runner)
	if err != nil {
		t.Fatalf("newExternalPDFExtractor: %v", err)
	}

	if _, err := extractor.Extract(t.Context(), []byte("pdf")); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if _, err := os.Stat(inputPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary input still exists: %v", err)
	}
}

func TestExternalPDFExtractorNonZeroExit(t *testing.T) {
	runner := fakePDFCommandRunner{run: func(_ context.Context, _ string, _ []string, _, stderr io.Writer) error {
		_, _ = io.WriteString(stderr, strings.Repeat("x", maxPDFConverterStderr+100))
		return fmt.Errorf("exit status 7")
	}}
	extractor, err := newExternalPDFExtractor("converter {input}", time.Second, runner)
	if err != nil {
		t.Fatalf("newExternalPDFExtractor: %v", err)
	}

	_, err = extractor.Extract(t.Context(), []byte("pdf"))
	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("error = %v", err)
	}
	if len(err.Error()) > maxPDFConverterStderr+200 {
		t.Fatalf("stderr was not bounded: error length %d", len(err.Error()))
	}
}

func TestExternalPDFExtractorRejectsStdoutOverflow(t *testing.T) {
	runner := fakePDFCommandRunner{run: func(_ context.Context, _ string, _ []string, stdout, _ io.Writer) error {
		chunk := strings.Repeat("x", 1024)
		for written := 0; written <= maxPDFConverterStdout; written += len(chunk) {
			if _, err := io.WriteString(stdout, chunk); err != nil {
				return err
			}
		}
		return nil
	}}
	extractor, err := newExternalPDFExtractor("converter {input}", time.Second, runner)
	if err != nil {
		t.Fatalf("newExternalPDFExtractor: %v", err)
	}

	_, err = extractor.Extract(t.Context(), []byte("pdf"))
	if err == nil || !strings.Contains(err.Error(), "converter output exceeds 10 MiB limit") {
		t.Fatalf("error = %v, want converter output limit error", err)
	}
}

func TestExternalPDFExtractorTimeout(t *testing.T) {
	runner := fakePDFCommandRunner{run: func(ctx context.Context, _ string, _ []string, _, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	extractor, err := newExternalPDFExtractor("converter {input}", 10*time.Millisecond, runner)
	if err != nil {
		t.Fatalf("newExternalPDFExtractor: %v", err)
	}

	_, err = extractor.Extract(t.Context(), []byte("pdf"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestExternalPDFExtractorRequiresOnePlaceholder(t *testing.T) {
	for _, command := range []string{"converter", "converter {input} {input}"} {
		t.Run(command, func(t *testing.T) {
			_, err := newExternalPDFExtractor(command, time.Second, fakePDFCommandRunner{})
			if err == nil || !strings.Contains(err.Error(), "exactly one {input}") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestExternalPDFExtractorRejectsEmptyStdout(t *testing.T) {
	runner := fakePDFCommandRunner{run: func(_ context.Context, _ string, _ []string, _, _ io.Writer) error {
		return nil
	}}
	extractor, err := newExternalPDFExtractor("converter {input}", time.Second, runner)
	if err != nil {
		t.Fatalf("newExternalPDFExtractor: %v", err)
	}

	_, err = extractor.Extract(t.Context(), []byte("pdf"))
	if err == nil || !strings.Contains(err.Error(), "empty output") {
		t.Fatalf("error = %v", err)
	}
}
