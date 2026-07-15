// Package cookies loads Netscape cookies.txt jars and matches entries
// against request URLs per RFC 6265 (§5.1.3 domain-match, §5.1.4 path-match).
// Cookie values are never included in errors, logs, or String output.
package cookies

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cookie is one live jar entry.
type Cookie struct {
	Domain   string    // lowercase, leading dot stripped
	HostOnly bool      // true: matches the exact host only
	Path     string    // always begins with "/"
	Secure   bool      // only sent over https
	HTTPOnly bool      // from the #HttpOnly_ line prefix
	Expires  time.Time // zero = session cookie (never expires for our purposes)
	Name     string
	Value    string
}

// Jar is an immutable, loaded cookie jar. All methods are nil-safe.
type Jar struct {
	cookies []Cookie
	Expired int // data lines skipped because their expiry had passed
}

// ExpandPath expands a leading "~/" to the user's home directory. On a
// UserHomeDir error, the original path is returned unchanged. Exported so
// callers that stat the jar (permission checks) resolve the same path Load does.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// Load opens the jar at path (expanding a leading "~/") and parses it.
// Only I/O errors are returned; malformed lines are skipped silently and
// never echoed.
func Load(path string) (*Jar, error) {
	f, err := os.Open(ExpandPath(path))
	if err != nil {
		return nil, fmt.Errorf("cookie file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

// lineResult classifies the outcome of parsing one jar line.
type lineResult int

const (
	lineSkip    lineResult = iota // blank, comment, malformed, or expired
	lineExpired                   // data line dropped because its expiry passed
	lineOK                        // a live cookie was produced
)

// Parse reads a Netscape cookies.txt stream and returns the live jar.
func Parse(r io.Reader) (*Jar, error) {
	jar := &Jar{}
	now := time.Now()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		c, res := parseLine(line, now)
		switch res {
		case lineExpired:
			jar.Expired++
		case lineOK:
			jar.cookies = append(jar.cookies, c)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("cookie file: %w", err)
	}
	return jar, nil
}

// parseLine parses one raw line into a Cookie. Malformed or expired lines are
// reported via lineResult; line content is never surfaced in errors.
func parseLine(line string, now time.Time) (Cookie, lineResult) {
	if strings.TrimSpace(line) == "" {
		return Cookie{}, lineSkip
	}

	httpOnly := false
	if strings.HasPrefix(line, "#HttpOnly_") {
		httpOnly = true
		line = strings.TrimPrefix(line, "#HttpOnly_")
	} else if strings.HasPrefix(line, "#") {
		return Cookie{}, lineSkip
	}

	fields := strings.SplitN(line, "\t", 7)
	if len(fields) != 7 {
		return Cookie{}, lineSkip
	}

	domain, ok := validDomain(fields[0])
	if !ok {
		return Cookie{}, lineSkip
	}
	includeSubdomains, ok := parseBool(fields[1])
	if !ok {
		return Cookie{}, lineSkip
	}
	// The include-subdomains field is authoritative. A contradictory leading
	// dot must never broaden a FALSE cookie to subdomains.
	hostOnly := !includeSubdomains

	path := fields[2]
	if !validPath(path) {
		return Cookie{}, lineSkip
	}

	secure, ok := parseBool(fields[3])
	if !ok {
		return Cookie{}, lineSkip
	}

	expiry, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil || expiry < 0 {
		return Cookie{}, lineSkip
	}
	var expires time.Time
	if expiry > 0 {
		expires = time.Unix(expiry, 0)
		if !expires.After(now) {
			return Cookie{}, lineExpired
		}
	}

	name := fields[5]
	if name == "" {
		return Cookie{}, lineSkip
	}
	value := fields[6]

	return Cookie{
		Domain:   domain,
		HostOnly: hostOnly,
		Path:     path,
		Secure:   secure,
		HTTPOnly: httpOnly,
		Expires:  expires,
		Name:     name,
		Value:    value,
	}, lineOK
}

func parseBool(field string) (bool, bool) {
	switch {
	case strings.EqualFold(field, "TRUE"):
		return true, true
	case strings.EqualFold(field, "FALSE"):
		return false, true
	default:
		return false, false
	}
}

// validDomain accepts IP literals and ASCII DNS hostnames. It strips one
// conventional leading dot only after validating that doing so cannot turn a
// malformed value into a broader domain.
func validDomain(field string) (string, bool) {
	if field == "" {
		return "", false
	}
	if strings.TrimSpace(field) != field {
		return "", false
	}
	domain := strings.TrimPrefix(field, ".")
	if domain == "" {
		return "", false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return "", false
	}
	domain = strings.ToLower(domain)
	if net.ParseIP(domain) != nil {
		return domain, true
	}
	if len(domain) > 253 {
		return "", false
	}
	for _, label := range strings.Split(domain, ".") {
		if !validDNSLabel(label) {
			return "", false
		}
	}
	return domain, true
}

func validDNSLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	if label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for i := 0; i < len(label); i++ {
		if !validDNSLabelByte(label[i]) {
			return false
		}
	}
	return true
}

func validDNSLabelByte(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-'
}

func validPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.ContainsAny(path, "\x00\r\n")
}

// Fingerprint returns a stable short digest of every currently live cookie's
// identity, value, and scope. It supports conservative cache isolation across
// redirects without exposing cookie values. Empty means there are no live
// cookies; the method is nil-safe.
func (j *Jar) Fingerprint() string {
	if j == nil {
		return ""
	}
	h := sha256.New()
	now := time.Now()
	count := 0
	for _, c := range j.cookies {
		if !c.Expires.IsZero() && !c.Expires.After(now) {
			continue
		}
		count++
		writeFingerprintField(h, c.Domain)
		writeFingerprintField(h, strconv.FormatBool(c.HostOnly))
		writeFingerprintField(h, c.Path)
		writeFingerprintField(h, strconv.FormatBool(c.Secure))
		writeFingerprintField(h, strconv.FormatBool(c.HTTPOnly))
		writeFingerprintField(h, c.Name)
		writeFingerprintField(h, c.Value)
		writeFingerprintField(h, strconv.FormatInt(c.Expires.Unix(), 10))
	}
	if count == 0 {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func writeFingerprintField(w io.Writer, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.Write(size[:])
	_, _ = io.WriteString(w, value)
}

// Len reports the cookie count that was live when the jar was loaded. Safe on
// a nil jar; For independently rechecks expiry before every request.
func (j *Jar) Len() int {
	if j == nil {
		return 0
	}
	return len(j.cookies)
}

// For returns the cookies matching u in RFC 6265 §5.4 serialization order
// (longest path first, ties in jar order). Nil for a nil jar or nil URL.
func (j *Jar) For(u *url.URL) []Cookie {
	if j == nil || u == nil {
		return nil
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	reqPath := u.EscapedPath()
	secureReq := u.Scheme == "https"
	now := time.Now()

	var matched []Cookie
	for _, c := range j.cookies {
		if !c.Expires.IsZero() && !c.Expires.After(now) {
			continue
		}
		if !domainMatch(host, c) {
			continue
		}
		if !pathMatch(reqPath, c) {
			continue
		}
		if c.Secure && !secureReq {
			continue
		}
		matched = append(matched, c)
	}
	sort.SliceStable(matched, func(a, b int) bool {
		return len(matched[a].Path) > len(matched[b].Path)
	})
	return matched
}

// domainMatch implements RFC 6265 §5.1.3.
func domainMatch(host string, c Cookie) bool {
	if host == c.Domain {
		return true
	}
	if c.HostOnly {
		return false
	}
	if net.ParseIP(host) != nil { // IPs never match a parent domain
		return false
	}
	return strings.HasSuffix(host, "."+c.Domain)
}

// pathMatch implements RFC 6265 §5.1.4.
func pathMatch(reqPath string, c Cookie) bool {
	if reqPath == "" {
		reqPath = "/"
	}
	if reqPath == c.Path {
		return true
	}
	if strings.HasPrefix(reqPath, c.Path) {
		return strings.HasSuffix(c.Path, "/") || reqPath[len(c.Path)] == '/'
	}
	return false
}
