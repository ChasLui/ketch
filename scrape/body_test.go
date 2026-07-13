package scrape

import (
	"strings"
	"testing"
)

func TestReadBoundedBodySilentlyTruncates(t *testing.T) {
	body, err := readBoundedBody(strings.NewReader(strings.Repeat("x", MaxBodyBytes+1024)))
	if err != nil {
		t.Fatalf("readBoundedBody: %v", err)
	}
	if len(body) != MaxBodyBytes {
		t.Fatalf("body length = %d, want %d", len(body), MaxBodyBytes)
	}
}
