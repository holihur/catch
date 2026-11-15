package api

import (
	"sync"
	"time"
)

// rateLimiter is a fixed-window per-project limiter for ingest traffic.
type rateLimiter struct {
	limit int
	mu    sync.Mutex
	items map[string]*window
	calls int
}

type window struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit, items: map[string]*window{}}
}

// allow consumes n events from the project's window.
func (l *rateLimiter) allow(key string, n int) bool {
	if l.limit <= 0 {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.calls%2048 == 0 {
		for k, w := range l.items {
			if now.After(w.reset) {
				delete(l.items, k)
			}
		}
	}
	w, ok := l.items[key]
	if !ok || now.After(w.reset) {
		w = &window{reset: now.Add(time.Minute)}
		l.items[key] = w
	}
	if w.count+n > l.limit {
		return false
	}
	w.count += n
	return true
}
