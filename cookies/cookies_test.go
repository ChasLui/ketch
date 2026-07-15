package cookies

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"
)

// tab builds a tab-joined jar line from fields.
func tab(fields ...string) string { return strings.Join(fields, "\t") }

func future() string { return fmt.Sprintf("%d", time.Now().Add(24*time.Hour).Unix()) }
func past() string   { return fmt.Sprintf("%d", time.Now().Add(-24*time.Hour).Unix()) }

func parseOne(t *testing.T, line string) *Jar {
	t.Helper()
	jar, err := Parse(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	return jar
}

func TestParsePlainValidLine(t *testing.T) {
	jar := parseOne(t, tab("example.com", "TRUE", "/", "TRUE", future(), "sid", "abc123")+"\n")
	if jar.Len() != 1 {
		t.Fatalf("Len = %d, want 1", jar.Len())
	}
	c := jar.cookies[0]
	if c.Domain != "example.com" || c.HostOnly || c.Path != "/" || !c.Secure || c.HTTPOnly || c.Name != "sid" || c.Value != "abc123" {
		t.Fatalf("cookie = %#v", c)
	}
	if c.Expires.IsZero() {
		t.Fatal("expected non-zero expiry")
	}
}

func TestParseHTTPOnlyPrefix(t *testing.T) {
	jar := parseOne(t, "#HttpOnly_"+tab("example.com", "TRUE", "/", "FALSE", "0", "sid", "v"))
	if jar.Len() != 1 || !jar.cookies[0].HTTPOnly {
		t.Fatalf("HttpOnly not parsed: %#v", jar.cookies)
	}
}

func TestParseCommentBlankCRLF(t *testing.T) {
	text := "# a comment\r\n\r\n" + tab("example.com", "TRUE", "/", "FALSE", "0", "sid", "v") + "\r\n"
	if jar := parseOne(t, text); jar.Len() != 1 {
		t.Fatalf("Len = %d, want 1", jar.Len())
	}
}

func TestParseMalformedLinesSkipped(t *testing.T) {
	lines := []string{
		tab("example.com", "TRUE", "/", "FALSE", "0", "sid"),         // 6 fields
		tab("example.com", "TRUE", "/", "FALSE", "0", "", "v"),       // empty name
		tab("", "TRUE", "/", "FALSE", "0", "sid", "v"),               // empty domain
		tab("example.com", "TRUE", "/", "FALSE", "notint", "s", "v"), // non-numeric expiry
		tab("example.com", "TRUE", "/", "FALSE", "-1", "s", "v"),     // invalid negative expiry
	}
	jar := parseOne(t, strings.Join(lines, "\n"))
	if jar.Len() != 0 || jar.Expired != 0 {
		t.Fatalf("Len=%d Expired=%d, want 0/0", jar.Len(), jar.Expired)
	}
}

func TestParseRejectsMalformedScopeFields(t *testing.T) {
	lines := []string{
		tab("example.com", "MAYBE", "/", "FALSE", "0", "subdomains", "v"),
		tab("example.com", "TRUE", "", "FALSE", "0", "empty_path", "v"),
		tab("example.com", "TRUE", "relative", "FALSE", "0", "relative_path", "v"),
		tab("example.com", "TRUE", "/", "MAYBE", "0", "secure", "v"),
		tab("bad..example.com", "TRUE", "/", "FALSE", "0", "empty_label", "v"),
		tab("-bad.example.com", "TRUE", "/", "FALSE", "0", "bad_label", "v"),
		tab("example.com.", "TRUE", "/", "FALSE", "0", "trailing_dot", "v"),
		tab("exa_mple.com", "TRUE", "/", "FALSE", "0", "bad_char", "v"),
		tab("..example.com", "TRUE", "/", "FALSE", "0", "double_dot", "v"),
	}
	jar := parseOne(t, strings.Join(lines, "\n"))
	if jar.Len() != 0 {
		t.Fatalf("Len = %d, want all malformed scope cookies rejected", jar.Len())
	}
}

func TestParseExpiredCountedAndSkipped(t *testing.T) {
	jar := parseOne(t, tab("example.com", "TRUE", "/", "FALSE", past(), "sid", "v"))
	if jar.Len() != 0 || jar.Expired != 1 {
		t.Fatalf("Len=%d Expired=%d, want 0/1", jar.Len(), jar.Expired)
	}
}

func TestForRechecksExpiryAfterLoad(t *testing.T) {
	// 1.5s leaves at least half a second after Unix-second truncation, avoiding
	// a false initial expiry on a busy test runner.
	expiry := time.Now().Add(1500 * time.Millisecond).Unix()
	jar := parseOne(t, tab("example.com", "FALSE", "/", "FALSE", fmt.Sprintf("%d", expiry), "sid", "v"))
	u, _ := url.Parse("https://example.com/")
	if len(jar.For(u)) != 1 || jar.Fingerprint() == "" {
		t.Fatal("cookie should match and isolate cache before its expiry")
	}
	if wait := time.Until(time.Unix(expiry, 0)); wait > 0 {
		time.Sleep(wait + 20*time.Millisecond)
	}
	if got := jar.For(u); len(got) != 0 {
		t.Fatalf("expired cookie still matched after load: %#v", got)
	}
	if jar.Fingerprint() != "" {
		t.Fatal("expired cookie still affected cache identity")
	}
}

func TestParseSessionCookieKept(t *testing.T) {
	jar := parseOne(t, tab("example.com", "TRUE", "/", "FALSE", "0", "sid", "v"))
	if jar.Len() != 1 || !jar.cookies[0].Expires.IsZero() {
		t.Fatalf("session cookie = %#v", jar.cookies)
	}
}

func TestParseValueWithLiteralTab(t *testing.T) {
	jar := parseOne(t, tab("example.com", "TRUE", "/", "FALSE", "0", "sid", "a\tb\tc"))
	if jar.Len() != 1 || jar.cookies[0].Value != "a\tb\tc" {
		t.Fatalf("value = %q", jar.cookies[0].Value)
	}
}

func TestParseHostOnlyConvention(t *testing.T) {
	dotFalse := parseOne(t, tab(".example.com", "FALSE", "/", "FALSE", "0", "sid", "v"))
	if !dotFalse.cookies[0].HostOnly {
		t.Error("FALSE include-subdomains must remain host-only despite a leading dot")
	}
	if dotFalse.cookies[0].Domain != "example.com" {
		t.Errorf("domain = %q, want example.com", dotFalse.cookies[0].Domain)
	}
	dotTrue := parseOne(t, tab(".example.com", "TRUE", "/", "FALSE", "0", "sid", "v"))
	if dotTrue.cookies[0].HostOnly {
		t.Error("TRUE include-subdomains should produce a domain cookie")
	}
	nodot := parseOne(t, tab("example.com", "FALSE", "/", "FALSE", "0", "sid", "v"))
	if !nodot.cookies[0].HostOnly {
		t.Error("no-dot domain with FALSE flag should be host-only")
	}
}

func TestParseDomainCaseFolded(t *testing.T) {
	jar := parseOne(t, tab(".Example.COM", "TRUE", "/", "FALSE", "0", "sid", "v"))
	if jar.cookies[0].Domain != "example.com" {
		t.Fatalf("domain = %q", jar.cookies[0].Domain)
	}
}

func TestDomainMatch(t *testing.T) {
	cases := []struct {
		host string
		c    Cookie
		want bool
	}{
		{"example.com", Cookie{Domain: "example.com"}, true},
		{"sub.example.com", Cookie{Domain: "example.com"}, true},
		{"sub.example.com", Cookie{Domain: "example.com", HostOnly: true}, false},
		{"notexample.com", Cookie{Domain: "example.com"}, false},
		{"127.0.0.1", Cookie{Domain: "0.0.1"}, false},
		{"127.0.0.1", Cookie{Domain: "127.0.0.1"}, true},
	}
	for _, tc := range cases {
		if got := domainMatch(tc.host, tc.c); got != tc.want {
			t.Errorf("domainMatch(%q, %+v) = %v, want %v", tc.host, tc.c, got, tc.want)
		}
	}

	// Trailing-dot host is stripped by For before matching.
	t.Run("trailing-dot host via For", func(t *testing.T) {
		jar, _ := Parse(strings.NewReader(tab("example.com", "TRUE", "/", "FALSE", "0", "sid", "v")))
		u, _ := url.Parse("http://example.com./")
		if len(jar.For(u)) != 1 {
			t.Fatal("trailing-dot host should match")
		}
	})
}

func TestPathMatch(t *testing.T) {
	cases := []struct {
		req  string
		path string
		want bool
	}{
		{"/anything", "/", true},
		{"/docs", "/docs", true},
		{"/docs/", "/docs", true},
		{"/docs/x", "/docs", true},
		{"/docsx", "/docs", false},
		{"/docs/x", "/docs/", true},
		{"/docs", "/docs/", false},
		{"", "/", true},
	}
	for _, tc := range cases {
		if got := pathMatch(tc.req, Cookie{Path: tc.path}); got != tc.want {
			t.Errorf("pathMatch(%q, %q) = %v, want %v", tc.req, tc.path, got, tc.want)
		}
	}
}

func TestForSecureAndOrder(t *testing.T) {
	lines := []string{
		tab("example.com", "TRUE", "/", "FALSE", "0", "short", "1"),
		tab("example.com", "TRUE", "/deep/path", "FALSE", "0", "long", "2"),
		tab("example.com", "TRUE", "/", "TRUE", "0", "secureonly", "3"),
	}
	jar, _ := Parse(strings.NewReader(strings.Join(lines, "\n")))

	t.Run("secure excluded over http", func(t *testing.T) {
		u, _ := url.Parse("http://example.com/deep/path")
		got := jar.For(u)
		for _, c := range got {
			if c.Name == "secureonly" {
				t.Fatal("secure cookie should not be sent over http")
			}
		}
	})

	t.Run("secure included over https and ordered longest path first", func(t *testing.T) {
		u, _ := url.Parse("https://example.com/deep/path")
		got := jar.For(u)
		if len(got) != 3 {
			t.Fatalf("got %d cookies, want 3", len(got))
		}
		if got[0].Name != "long" {
			t.Fatalf("first cookie = %q, want long (longest path)", got[0].Name)
		}
	})

	t.Run("port ignored", func(t *testing.T) {
		u, _ := url.Parse("https://example.com:8443/deep/path")
		if len(jar.For(u)) != 3 {
			t.Fatal("port should be ignored in host matching")
		}
	})
}

func TestNilSafety(t *testing.T) {
	var j *Jar
	u, _ := url.Parse("https://example.com/")
	if j.For(u) != nil {
		t.Error("nil jar For should return nil")
	}
	if j.Len() != 0 {
		t.Error("nil jar Len should be 0")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/to/cookies.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "cookie file") {
		t.Fatalf("error = %q, want cookie file prefix", err)
	}
}
