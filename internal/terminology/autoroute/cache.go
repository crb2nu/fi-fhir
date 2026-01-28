package autoroute

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// Cache provides in-memory caching for autoroute results.
// It uses a simple LRU-like eviction based on TTL.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
	maxSize int
}

type cacheEntry struct {
	result    *SuggestResult
	expiresAt time.Time
}

// CacheConfig configures the autoroute cache.
type CacheConfig struct {
	TTL     time.Duration // How long entries remain valid (default: 15 minutes)
	MaxSize int           // Maximum entries (default: 10000)
}

// DefaultCacheConfig returns default cache configuration.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:     15 * time.Minute,
		MaxSize: 10000,
	}
}

// NewCache creates a new autoroute result cache.
func NewCache(cfg CacheConfig) *Cache {
	if cfg.TTL == 0 {
		cfg.TTL = 15 * time.Minute
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 10000
	}

	c := &Cache{
		entries: make(map[string]*cacheEntry),
		ttl:     cfg.TTL,
		maxSize: cfg.MaxSize,
	}

	// Start background cleanup
	go c.cleanupLoop()

	return c
}

// Get retrieves a cached result if available and not expired.
func (c *Cache) Get(req SuggestRequest) *SuggestResult {
	key := c.key(req)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil
	}

	if time.Now().After(entry.expiresAt) {
		// Entry expired, remove it
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil
	}

	return entry.result
}

// Set stores a result in the cache.
func (c *Cache) Set(req SuggestRequest, result *SuggestResult) {
	key := c.key(req)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict oldest entries if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes a specific entry from the cache.
func (c *Cache) Invalidate(req SuggestRequest) {
	key := c.key(req)

	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]*cacheEntry)
	c.mu.Unlock()
}

// Size returns the current number of cached entries.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// key generates a deterministic cache key from a request.
func (c *Cache) key(req SuggestRequest) string {
	// Create a deterministic hash of the request parameters
	h := sha256.New()
	h.Write([]byte(req.SourceSystem))
	h.Write([]byte{0})
	h.Write([]byte(req.SourceCode))
	h.Write([]byte{0})
	h.Write([]byte(req.TargetSystem))
	h.Write([]byte{0})
	h.Write([]byte(req.ProfileID))
	return "autoroute:" + hex.EncodeToString(h.Sum(nil))[:16]
}

// evictOldest removes the oldest entries to make room for new ones.
// Must be called with lock held.
func (c *Cache) evictOldest() {
	// Simple strategy: remove 10% of entries or expired ones
	toRemove := c.maxSize / 10
	if toRemove < 1 {
		toRemove = 1
	}

	now := time.Now()
	removed := 0

	// First pass: remove expired entries
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
			removed++
		}
		if removed >= toRemove {
			return
		}
	}

	// Second pass: remove oldest entries (by expiration time)
	if removed < toRemove {
		var oldestKey string
		var oldestExpiry time.Time

		for key, entry := range c.entries {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = key
				oldestExpiry = entry.expiresAt
			}
		}

		if oldestKey != "" {
			delete(c.entries, oldestKey)
		}
	}
}

// cleanupLoop periodically removes expired entries.
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes all expired entries.
func (c *Cache) cleanup() {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}
