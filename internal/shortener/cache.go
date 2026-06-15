package shortener

import (
	"sync"
	"time"
)

type Cache struct {
	mu         sync.RWMutex
	items      map[string]cacheEntry
	maxEntries int
}

type cacheEntry struct {
	originalURL string
	expiresAt   *time.Time
	cachedAt    time.Time
}

func NewCache() *Cache {
	return &Cache{items: make(map[string]cacheEntry), maxEntries: 1024}
}

func (c *Cache) Get(alias string, now time.Time) (string, bool) {
	c.mu.RLock()
	entry, ok := c.items[alias]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if isExpired(entry.expiresAt, now) {
		c.Delete(alias)
		return "", false
	}
	return entry.originalURL, true
}

func (c *Cache) Set(alias, originalURL string, expiresAt *time.Time, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[alias]; !ok && len(c.items) >= c.maxEntries {
		c.deleteOldestLocked()
	}
	c.items[alias] = cacheEntry{originalURL: originalURL, expiresAt: expiresAt, cachedAt: now}
}

func (c *Cache) Delete(alias string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, alias)
}

func (c *Cache) deleteOldestLocked() {
	var oldestAlias string
	var oldestTime time.Time
	first := true
	for alias, entry := range c.items {
		if first || entry.cachedAt.Before(oldestTime) {
			oldestAlias = alias
			oldestTime = entry.cachedAt
			first = false
		}
	}
	delete(c.items, oldestAlias)
}
