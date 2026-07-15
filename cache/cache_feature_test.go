package cache

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/1broseidon/ketch/scrape"
)

func newTestCache(t *testing.T, ttl time.Duration) *Cache {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := NewBBoltStore(path)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Cache{store: store, ttl: ttl}
}

func TestFeaturePutThenGet(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 1*time.Hour)

	page := &scrape.Page{
		URL:      "https://example.com/test",
		Title:    "Test Page",
		Markdown: "# Hello\n\nWorld",
	}
	c.Put("https://example.com/test", page, scrape.SourceHTTP)

	got, source := c.Get("https://example.com/test")
	if got == nil {
		t.Fatal("expected cached page, got nil")
	}
	if source != scrape.SourceHTTP {
		t.Errorf("source = %q, want %q", source, scrape.SourceHTTP)
	}
	if got.Title != "Test Page" {
		t.Errorf("title = %q, want %q", got.Title, "Test Page")
	}
	if got.Markdown != "# Hello\n\nWorld" {
		t.Errorf("markdown = %q, want %q", got.Markdown, "# Hello\n\nWorld")
	}
	if got.URL != "https://example.com/test" {
		t.Errorf("url = %q, want %q", got.URL, "https://example.com/test")
	}
}

func TestFeatureSourceRoundTrip(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 1*time.Hour)

	page := &scrape.Page{URL: "https://example.com/browser", Title: "Rendered"}
	c.Put("https://example.com/browser", page, scrape.SourceBrowser)

	got, source := c.Get("https://example.com/browser")
	if got == nil {
		t.Fatal("expected cached page, got nil")
	}
	if source != scrape.SourceBrowser {
		t.Errorf("source = %q, want %q", source, scrape.SourceBrowser)
	}
}

func TestFeatureGetExpiredTTL(t *testing.T) {
	t.Parallel()
	// Use a TTL of 1 nanosecond — entries expire immediately
	c := newTestCache(t, 1*time.Nanosecond)

	page := &scrape.Page{URL: "https://example.com/expired", Title: "Old"}
	c.Put("https://example.com/expired", page, scrape.SourceHTTP)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	got, _ := c.Get("https://example.com/expired")
	if got != nil {
		t.Error("expected nil for expired entry, got non-nil")
	}
}

func TestFeatureGetValidTTL(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 24*time.Hour)

	page := &scrape.Page{URL: "https://example.com/valid", Title: "Fresh"}
	c.Put("https://example.com/valid", page, scrape.SourceHTTP)

	got, _ := c.Get("https://example.com/valid")
	if got == nil {
		t.Fatal("expected cached page within TTL, got nil")
	}
	if got.Title != "Fresh" {
		t.Errorf("title = %q, want %q", got.Title, "Fresh")
	}
}

func TestFeatureStats(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 1*time.Hour)

	// Empty cache
	entries, size := c.Stats()
	if entries != 0 {
		t.Errorf("empty cache entries = %d, want 0", entries)
	}
	if size <= 0 {
		// bbolt always has some overhead, but size should be positive
		t.Logf("empty cache size = %d (bbolt overhead)", size)
	}

	// Add entries
	c.Put("https://example.com/a", &scrape.Page{URL: "https://example.com/a", Title: "A"}, scrape.SourceHTTP)
	c.Put("https://example.com/b", &scrape.Page{URL: "https://example.com/b", Title: "B"}, scrape.SourceHTTP)

	entries, _ = c.Stats()
	if entries != 2 {
		t.Errorf("entries after 2 puts = %d, want 2", entries)
	}
}

func TestFeatureClear(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 1*time.Hour)

	c.Put("https://example.com/x", &scrape.Page{URL: "https://example.com/x", Title: "X"}, scrape.SourceHTTP)
	c.Put("https://example.com/y", &scrape.Page{URL: "https://example.com/y", Title: "Y"}, scrape.SourceHTTP)

	if err := c.Clear(); err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}

	entries, _ := c.Stats()
	if entries != 0 {
		t.Errorf("entries after clear = %d, want 0", entries)
	}

	// Verify get returns nil after clear
	if got, _ := c.Get("https://example.com/x"); got != nil {
		t.Error("expected nil after clear")
	}
}

func TestFeatureNilCacheReceiver(t *testing.T) {
	t.Parallel()
	var c *Cache

	// None of these should panic
	if got, _ := c.Get("https://example.com"); got != nil {
		t.Error("nil cache Get should return nil")
	}

	c.Put("https://example.com", &scrape.Page{URL: "u"}, scrape.SourceHTTP) // should not panic

	entries, size := c.Stats()
	if entries != 0 || size != 0 {
		t.Errorf("nil cache Stats = (%d, %d), want (0, 0)", entries, size)
	}

	if err := c.Clear(); err != nil {
		t.Errorf("nil cache Clear returned error: %v", err)
	}

	c.Close() // should not panic
}

func TestFeatureDifferentURLsDifferentKeys(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 1*time.Hour)

	page1 := &scrape.Page{URL: "https://example.com/page1", Title: "Page 1", Markdown: "Content 1"}
	page2 := &scrape.Page{URL: "https://example.com/page2", Title: "Page 2", Markdown: "Content 2"}

	c.Put("https://example.com/page1", page1, scrape.SourceHTTP)
	c.Put("https://example.com/page2", page2, scrape.SourceHTTP)

	got1, _ := c.Get("https://example.com/page1")
	got2, _ := c.Get("https://example.com/page2")

	if got1 == nil || got2 == nil {
		t.Fatal("both pages should be cached")
	}
	if got1.Title != "Page 1" {
		t.Errorf("page1 title = %q, want %q", got1.Title, "Page 1")
	}
	if got2.Title != "Page 2" {
		t.Errorf("page2 title = %q, want %q", got2.Title, "Page 2")
	}
}

func TestFeatureGetMissingKey(t *testing.T) {
	t.Parallel()
	c := newTestCache(t, 1*time.Hour)

	got, _ := c.Get("https://example.com/nonexistent")
	if got != nil {
		t.Error("expected nil for missing key")
	}
}

func TestCachePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	t.Run("new database is private", testNewDatabasePermissions)
	t.Run("existing database is tightened on open", testExistingDatabasePermissions)
	t.Run("cache directory is private", testCacheDirectoryPermissions)
}

func testNewDatabasePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	store, err := NewBBoltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertMode(t, path, 0o600)
}

func testExistingDatabasePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	store, err := NewBBoltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err = NewBBoltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertMode(t, path, 0o600)
}

func testCacheDirectoryPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ketch")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	assertMode(t, dir, 0o700)
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode = %o, want %o", got, want)
	}
}

// Ensure the DB file is actually created on disk
func TestFeatureDBFileCreated(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.db")
	// sub directory doesn't exist yet — NewBBoltStore should handle it
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewBBoltStore(path)
	if err != nil {
		t.Fatalf("NewBBoltStore: %v", err)
	}
	defer store.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected DB file to exist after creating store")
	}
}
