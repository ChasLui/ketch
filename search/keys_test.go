package search

import (
	"reflect"
	"testing"
)

func TestKeyPoolEmptyOneMany(t *testing.T) {
	empty := newKeyPool(nil)
	if empty.size() != 0 || empty.pick() != "" {
		t.Fatalf("empty pool: size=%d pick=%q", empty.size(), empty.pick())
	}

	one := newKeyPool([]string{" only "})
	if one.size() != 1 || one.pick() != "only" {
		t.Fatalf("one-key pool: size=%d pick=%q", one.size(), one.pick())
	}

	many := newKeyPool([]string{"one", "two", "one", " ", "three"})
	if many.size() != 3 || !reflect.DeepEqual(many.keys, []string{"one", "two", "three"}) {
		t.Fatalf("many-key pool = %v", many.keys)
	}
}

func TestKeyPoolInjectedDistribution(t *testing.T) {
	pool := newKeyPool([]string{"one", "two", "three"})
	next := 0
	pool.randIntN = func(n int) int {
		value := next % n
		next++
		return value
	}

	counts := map[string]int{}
	for range 300 {
		counts[pool.pick()]++
	}
	for _, key := range pool.keys {
		if counts[key] != 100 {
			t.Fatalf("distribution = %v, want 100 picks per key", counts)
		}
	}
}

func TestKeyPoolPickDifferent(t *testing.T) {
	pool := newKeyPool([]string{"one", "two", "three"})
	pool.randIntN = func(int) int { return 0 }
	if got := pool.pickDifferent("one"); got != "two" {
		t.Fatalf("pickDifferent(one) = %q, want two", got)
	}
	if got := pool.pickDifferent("two"); got != "one" {
		t.Fatalf("pickDifferent(two) = %q, want one", got)
	}
}
