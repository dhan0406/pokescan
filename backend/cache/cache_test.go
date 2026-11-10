package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// clock is a controllable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestCache_hitBeforeTTL(t *testing.T) {
	clk := newClock()
	c := New[string](time.Minute, clk.Now)
	c.Set("a", "apple")
	clk.Advance(59 * time.Second)
	got, ok := c.Get("a")
	if !ok || got != "apple" {
		t.Errorf("Get(a) = %q, %v, want apple, true", got, ok)
	}
}

func TestCache_missAfterTTL(t *testing.T) {
	clk := newClock()
	c := New[string](time.Minute, clk.Now)
	c.Set("a", "apple")
	clk.Advance(time.Minute + time.Second)
	got, ok := c.Get("a")
	if ok || got != "" {
		t.Errorf("Get(a) = %q, %v, want zero value, false", got, ok)
	}
}

func TestCache_missingKey(t *testing.T) {
	c := New[int](time.Minute, newClock().Now)
	if v, ok := c.Get("nope"); ok || v != 0 {
		t.Errorf("Get(nope) = %d, %v, want 0, false", v, ok)
	}
}

func TestCache_expiredEntryIsRemoved(t *testing.T) {
	clk := newClock()
	c := New[string](time.Minute, clk.Now)
	c.Set("a", "apple")
	clk.Advance(2 * time.Minute)
	c.Get("a")
	if n := len(c.items); n != 0 {
		t.Errorf("%d items left after expired Get, want 0", n)
	}
}

func TestCache_overwriteResetsTTL(t *testing.T) {
	clk := newClock()
	c := New[string](time.Minute, clk.Now)
	c.Set("a", "old")
	clk.Advance(50 * time.Second)
	c.Set("a", "new")
	clk.Advance(50 * time.Second) // 100s after first Set, 50s after second
	got, ok := c.Get("a")
	if !ok || got != "new" {
		t.Errorf("Get(a) = %q, %v, want new, true", got, ok)
	}
}

func TestCache_concurrentAccess(t *testing.T) {
	c := New[int](time.Minute, time.Now)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%5)
			c.Set(key, i)
			c.Get(key)
		}(i)
	}
	wg.Wait()
}
