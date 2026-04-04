package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cleanupIntervalRequests = 128

type rateLimitBucket struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

type rateLimiter struct {
	window   time.Duration
	limit    int
	mu       sync.Mutex
	buckets  map[string]rateLimitBucket
	requests uint64
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
	l.requests++
	if l.requests%cleanupIntervalRequests == 0 {
		l.cleanupExpiredLocked(now)
	}
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
	if remoteIP, ok := requestIPFromRemoteAddr(r.RemoteAddr); ok {
		if trustedProxy(remoteIP) {
			if forwarded := forwardedClientIP(r.Header.Get("X-Forwarded-For")); forwarded != "" {
				return forwarded + ":" + r.URL.Path
			}
		}
		return remoteIP + ":" + r.URL.Path
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr + ":" + r.URL.Path
	}
	return "unknown:" + r.URL.Path
}

func forwardedClientIP(header string) string {
	for _, part := range strings.Split(header, ",") {
		candidate := strings.TrimSpace(part)
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func requestIPFromRemoteAddr(remoteAddr string) (string, bool) {
	if remoteAddr == "" {
		return "", false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host, true
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String(), true
	}
	return "", false
}

func trustedProxy(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
