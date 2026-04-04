package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimitBucket struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

type rateLimiter struct {
	window  time.Duration
	limit   int
	mu      sync.Mutex
	buckets map[string]rateLimitBucket
}

func newRateLimiter(window time.Duration, limit int) *rateLimiter {
	return &rateLimiter{
		window:  window,
		limit:   limit,
		buckets: make(map[string]rateLimitBucket),
	}
}

func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucket := l.buckets[key]
	if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= l.window {
		bucket.windowStart = now
		bucket.count = 0
	}

	bucket.count++
	bucket.lastSeen = now
	l.buckets[key] = bucket
	l.cleanupExpiredLocked(now)
	return bucket.count <= l.limit
}

func (l *rateLimiter) cleanupExpiredLocked(now time.Time) {
	cutoff := now.Add(-2 * l.window)
	for key, bucket := range l.buckets {
		if bucket.lastSeen.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}

func RateLimit(window time.Duration, limit int) func(http.Handler) http.Handler {
	limiter := newRateLimiter(window, limit)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.allow(rateLimitKey(r), time.Now()) {
				writeJSONError(w, http.StatusTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func rateLimitKey(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded + ":" + r.URL.Path
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host + ":" + r.URL.Path
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr + ":" + r.URL.Path
	}
	return "unknown:" + r.URL.Path
}
