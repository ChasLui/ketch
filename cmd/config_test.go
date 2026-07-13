package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/1broseidon/ketch/config"
)

func TestApplyConfigSetURLRewritesValidJSON(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "url_rewrites", `[{"match":"^https?://www\\.reddit\\.com/(.*)$","replace":"https://old.reddit.com/$1"}]`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(c.URLRewrites) != 1 {
		t.Fatalf("want 1 rule, got %d", len(c.URLRewrites))
	}
	if c.URLRewrites[0].Replace != "https://old.reddit.com/$1" {
		t.Errorf("Replace mismatch: %q", c.URLRewrites[0].Replace)
	}
}

func TestApplyConfigSetURLRewritesInvalidJSON(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "url_rewrites", `not json`)
	if err == nil {
		t.Fatalf("want JSON parse error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "json") {
		t.Errorf("error should mention JSON, got: %v", err)
	}
}

func TestApplyConfigSetURLRewritesInvalidRegex(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "url_rewrites", `[{"match":"[","replace":"x"}]`)
	if err == nil {
		t.Fatalf("want regex compile error")
	}
}

func TestApplyConfigSetURLRewritesEmptyClears(t *testing.T) {
	c := config.Defaults()
	if err := applyConfigSet(&c, "url_rewrites", `[{"match":"^x$","replace":"y"}]`); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(c.URLRewrites) != 1 {
		t.Fatalf("setup failed: %d rules", len(c.URLRewrites))
	}
	if err := applyConfigSet(&c, "url_rewrites", `[]`); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(c.URLRewrites) != 0 {
		t.Errorf("want empty list after [] reset, got %d rules", len(c.URLRewrites))
	}
}

func TestApplyConfigSetUnknownKey(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "no_such_key", "x")
	if err == nil {
		t.Fatalf("want unknown-key error")
	}
}

func TestApplyConfigSetSPAMarkersValid(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "spa_markers", `["__next_f","data-v-app"]`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(c.SPAMarkers) != 2 {
		t.Fatalf("want 2 markers, got %d", len(c.SPAMarkers))
	}
	if c.SPAMarkers[0] != "__next_f" || c.SPAMarkers[1] != "data-v-app" {
		t.Errorf("markers mismatch: %v", c.SPAMarkers)
	}
}

func TestApplyConfigSetSPAMarkersInvalidJSON(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "spa_markers", `not json`)
	if err == nil {
		t.Fatalf("want JSON parse error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "json") {
		t.Errorf("error should mention JSON, got: %v", err)
	}
}

func TestApplyConfigSetSPAMarkersRejectsBlank(t *testing.T) {
	c := config.Defaults()
	err := applyConfigSet(&c, "spa_markers", `["__next_f","  "]`)
	if err == nil {
		t.Fatalf("want blank-marker error")
	}
}

func TestApplyConfigSetSPAMarkersEmptyClears(t *testing.T) {
	c := config.Defaults()
	if err := applyConfigSet(&c, "spa_markers", `["__next_f"]`); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if len(c.SPAMarkers) != 1 {
		t.Fatalf("setup failed: %d markers", len(c.SPAMarkers))
	}
	if err := applyConfigSet(&c, "spa_markers", `[]`); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(c.SPAMarkers) != 0 {
		t.Errorf("want empty list after [] reset, got %d markers", len(c.SPAMarkers))
	}
}

func TestApplyConfigSetExternalPDFConverter(t *testing.T) {
	c := config.Defaults()
	if c.ExternalPDFToMDConverterTimeoutSec != 300 {
		t.Fatalf("default timeout = %d, want 300", c.ExternalPDFToMDConverterTimeoutSec)
	}
	if err := applyConfigSet(&c, "external_pdf_to_md_converter_command", "pdftotext {input} -"); err != nil {
		t.Fatalf("set command: %v", err)
	}
	if err := applyConfigSet(&c, "external_pdf_to_md_converter_timeout_sec", "45"); err != nil {
		t.Fatalf("set timeout: %v", err)
	}
	if c.ExternalPDFToMDConverterCommand != "pdftotext {input} -" || c.ExternalPDFToMDConverterTimeoutSec != 45 {
		t.Fatalf("config = %#v", c)
	}
}

func TestApplyConfigSetExternalPDFConverterRejectsInvalidCommand(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "invalid shlex", value: `converter "{input}`},
		{name: "missing placeholder", value: "converter input.pdf"},
		{name: "duplicate placeholder", value: "converter {input} --again={input}"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := config.Defaults()
			c.ExternalPDFToMDConverterCommand = "existing {input}"
			err := applyConfigSet(&c, "external_pdf_to_md_converter_command", test.value)
			if err == nil {
				t.Fatal("expected validation error")
			}
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != ExitValidation {
				t.Fatalf("error = %v, want exit %d", err, ExitValidation)
			}
			if c.ExternalPDFToMDConverterCommand != "existing {input}" {
				t.Fatalf("invalid value changed config to %q", c.ExternalPDFToMDConverterCommand)
			}
		})
	}
}

func TestApplyConfigSetExternalPDFConverterEmptyClears(t *testing.T) {
	c := config.Defaults()
	c.ExternalPDFToMDConverterCommand = "converter {input}"
	if err := applyConfigSet(&c, "external_pdf_to_md_converter_command", ""); err != nil {
		t.Fatalf("clear command: %v", err)
	}
	if c.ExternalPDFToMDConverterCommand != "" {
		t.Fatalf("command = %q, want empty", c.ExternalPDFToMDConverterCommand)
	}
}

func TestApplyConfigSetExternalPDFConverterRejectsInvalidTimeout(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-an-int"} {
		t.Run(value, func(t *testing.T) {
			c := config.Defaults()
			err := applyConfigSet(&c, "external_pdf_to_md_converter_timeout_sec", value)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
