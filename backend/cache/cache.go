// Package cache provides an in-memory TTL cache and cached wrappers for the
// upstream API clients.
package cache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	value   V
	expires time.Time
}

// Cache is a concurrency-safe in-memory store whose entries expire after a TTL.
// Expired entries are removed when they are read.
type Cache[V any] struct {
	ttl time.Duration
	now func() time.Time

	mu    sync.Mutex
	items map[string]entry[V]
}

// New returns a Cache whose entries live for ttl. now supplies the current time
// so tests can control the clock.
func New[V any](ttl time.Duration, now func() time.Time) *Cache[V] {
	return &Cache[V]{ttl: ttl, now: now, items: map[string]entry[V]{}}
}

// Get returns the value for key if it is present and not expired.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	if !c.now().Before(e.expires) {
		delete(c.items, key)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores v under key, replacing any existing entry and restarting its TTL.
func (c *Cache[V]) Set(key string, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = entry[V]{value: v, expires: c.now().Add(c.ttl)}
}
