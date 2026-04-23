package util

import (
	"sync"
	"time"
)

// slidingEntry tracks a list of request timestamps for one key.
type slidingEntry struct {
	mu         sync.Mutex
	timestamps []time.Time
}

// SlidingWindowRateLimiter is a thread-safe in-memory sliding-window rate limiter.
// It tracks requests per key (e.g. email address) within a rolling time window.
type SlidingWindowRateLimiter struct {
	mu       sync.RWMutex
	entries  map[string]*slidingEntry
	limit    int
	window   time.Duration
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewSlidingWindowRateLimiter creates a rate limiter that allows at most `limit`
// requests per `window` per key. It starts a background goroutine that prunes
// stale entries every `window` interval.
func NewSlidingWindowRateLimiter(limit int, window time.Duration) *SlidingWindowRateLimiter {
	rl := &SlidingWindowRateLimiter{
		entries: make(map[string]*slidingEntry),
		limit:   limit,
		window:  window,
		stopCh:  make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

// Allow returns true if the request is permitted, false if rate-limited.
// It always records the attempt (whether permitted or not) so that callers
// cannot bypass by retrying immediately after a rejection.
func (rl *SlidingWindowRateLimiter) Allow(key string) bool {
	entry := rl.getOrCreate(key)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove timestamps outside the window
	valid := entry.timestamps[:0]
	for _, t := range entry.timestamps {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	entry.timestamps = valid

	if len(entry.timestamps) >= rl.limit {
		return false
	}

	entry.timestamps = append(entry.timestamps, now)
	return true
}

// RemainingSeconds returns the number of seconds until the oldest in-window
// request falls outside the window, giving the caller retry-after info.
func (rl *SlidingWindowRateLimiter) RemainingSeconds(key string) int {
	entry := rl.getOrCreate(key)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if len(entry.timestamps) == 0 {
		return 0
	}

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Find oldest timestamp still in window
	var oldest time.Time
	for _, t := range entry.timestamps {
		if t.After(cutoff) {
			if oldest.IsZero() || t.Before(oldest) {
				oldest = t
			}
		}
	}
	if oldest.IsZero() {
		return 0
	}
	retryAt := oldest.Add(rl.window)
	secs := int(retryAt.Sub(now).Seconds())
	if secs < 0 {
		return 0
	}
	return secs
}

func (rl *SlidingWindowRateLimiter) getOrCreate(key string) *slidingEntry {
	rl.mu.RLock()
	e, ok := rl.entries[key]
	rl.mu.RUnlock()
	if ok {
		return e
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	// Double-check after acquiring write lock
	if e, ok = rl.entries[key]; ok {
		return e
	}
	e = &slidingEntry{}
	rl.entries[key] = e
	return e
}

// Stop halts the background cleanup goroutine. Safe to call multiple times.
func (rl *SlidingWindowRateLimiter) Stop() {
	rl.stopOnce.Do(func() { close(rl.stopCh) })
}

// cleanupLoop removes keys with no recent activity to prevent unbounded growth.
func (rl *SlidingWindowRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			rl.prune()
		}
	}
}

func (rl *SlidingWindowRateLimiter) prune() {
	now := time.Now()
	cutoff := now.Add(-rl.window)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, entry := range rl.entries {
		entry.mu.Lock()
		valid := entry.timestamps[:0]
		for _, t := range entry.timestamps {
			if t.After(cutoff) {
				valid = append(valid, t)
			}
		}
		entry.timestamps = valid
		if len(entry.timestamps) == 0 {
			delete(rl.entries, key)
		}
		entry.mu.Unlock()
	}
}
