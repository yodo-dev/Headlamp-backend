package util

import (
	"sync"
	"time"
)

type ttlCacheEntry struct {
	value     []byte
	expiresAt time.Time
}

// TTLCache stores byte payloads by key until expiry.
type TTLCache struct {
	mu      sync.RWMutex
	entries map[string]ttlCacheEntry
}

func NewTTLCache() *TTLCache {
	return &TTLCache{entries: make(map[string]ttlCacheEntry)}
}

func (c *TTLCache) Set(key string, value []byte, ttl time.Duration) {
	if c == nil {
		return
	}

	expiresAt := time.Now().UTC().Add(ttl)
	copied := append([]byte(nil), value...)

	c.mu.Lock()
	c.entries[key] = ttlCacheEntry{value: copied, expiresAt: expiresAt}
	c.mu.Unlock()
}

func (c *TTLCache) Get(key string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}

	now := time.Now().UTC()

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if now.After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return append([]byte(nil), entry.value...), true
}

func (c *TTLCache) Flush() {
	if c == nil {
		return
	}

	c.mu.Lock()
	c.entries = make(map[string]ttlCacheEntry)
	c.mu.Unlock()
}
