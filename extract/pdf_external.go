package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/google/shlex"
)

const (
	maxPDFConverterStdout = 10 << 20
	maxPDFConverterStderr = 4 << 10
)

// NewExternalPDFExtractor creates a PDF extractor that invokes command
// directly (without a shell). The shlex-parsed template must contain exactly
// one {input} placeholder, which is replaced with a temporary PDF path.
func NewExternalPDFExtractor(command string, timeout time.Duration) (PDFExtractor, error) {
	return newExternalPDFExtractor(command, timeout, execPDFCommandRunner{})
}

type externalPDFExtractor struct {
	command []string
	timeout time.Duration
	runner  pdfCommandRunner
}

type pdfCommandRunner interface {
	Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error
}

type execPDFCommandRunner struct{}

func (execPDFCommandRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func newExternalPDFExtractor(command string, timeout time.Duration, runner pdfCommandRunner) (*externalPDFExtractor, error) {
	argv, err := shlex.Split(command)
	if err != nil {
		return nil, fmt.Errorf("parse external PDF converter command: %w", err)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("external PDF converter command is empty")
	}
	placeholderCount := 0
	for _, arg := range argv {
		placeholderCount += strings.Count(arg, "{input}")
	}
	if placeholderCount != 1 {
		return nil, fmt.Errorf("external PDF converter command must contain exactly one {input} placeholder (found %d)", placeholderCount)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("external PDF converter timeout must be positive")
	}
	return &externalPDFExtractor{command: argv, timeout: timeout, runner: runner}, nil
}

func (e *externalPDFExtractor) Extract(ctx context.Context, src []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	input, err := os.CreateTemp("", "ketch-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temporary PDF: %w", err)
	}
	inputPath := input.Name()
	defer func() { _ = os.Remove(inputPath) }()

	if _, err := input.Write(src); err != nil {
		_ = input.Close()
		return "", fmt.Errorf("write temporary PDF: %w", err)
	}
	if err := input.Close(); err != nil {
		return "", fmt.Errorf("close temporary PDF: %w", err)
	}

	argv := make([]string, len(e.command))
	for i, arg := range e.command {
		argv[i] = strings.Replace(arg, "{input}", inputPath, 1)
	}

	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	stdout := &boundedBuffer{limit: maxPDFConverterStdout}
	stderr := &boundedBuffer{limit: maxPDFConverterStderr}
	runErr := e.runner.Run(runCtx, argv[0], argv[1:], stdout, stderr)
	if stdout.Overflowed() {
		return "", fmt.Errorf("external PDF converter output exceeds %d MiB limit", maxPDFConverterStdout>>20)
	}
	if runErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if ctxErr := runCtx.Err(); ctxErr != nil {
			return "", fmt.Errorf("external PDF converter timed out after %s: %w", e.timeout, ctxErr)
		}
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return "", fmt.Errorf("external PDF converter failed: %w: %s", runErr, detail)
		}
		return "", fmt.Errorf("external PDF converter failed: %w", runErr)
	}

	markdown := strings.TrimSpace(stdout.String())
	if markdown == "" {
		return "", fmt.Errorf("external PDF converter returned empty output")
	}
	return markdown, nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	remaining := b.limit - b.buffer.Len()
	if len(p) > remaining {
		b.overflow = true
	}
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = b.buffer.Write(p[:remaining])
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	return b.buffer.String()
}

func (b *boundedBuffer) Overflowed() bool {
	return b.overflow
}
