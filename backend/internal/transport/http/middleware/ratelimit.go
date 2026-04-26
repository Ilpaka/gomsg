package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"goflow/backend/internal/pkg/response"
)

// ClientIP returns a coarse client identifier from the request (host part of RemoteAddr).
func ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	addr := strings.TrimSpace(r.RemoteAddr)
	if addr == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" {
		return "unknown"
	}
	return host
}

// IPRateLimiter is a token-bucket limiter keyed by client IP (see ClientIP).
type IPRateLimiter struct {
	log   *slog.Logger
	name  string
	limit rate.Limit
	burst int

	mu   sync.Mutex
	byIP map[string]*ipLimiterEntry
}

type ipLimiterEntry struct {
	lim  *rate.Limiter
	last time.Time
}

// NewIPRateLimiter builds a per-IP limiter. rpm is sustained requests per minute (average).
// burst caps short bursts; if burst <= 0, a default is chosen from rpm.
// If rpm <= 0, returns nil (no limiting — caller should skip wrapping).
func NewIPRateLimiter(log *slog.Logger, name string, rpm int, burst int) *IPRateLimiter {
	if rpm <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = rpm / 3
		if burst < 1 {
			burst = 1
		}
		if burst > 60 {
			burst = 60
		}
	}
	lim := rate.Limit(float64(rpm) / 60.0)
	return &IPRateLimiter{
		log:   log,
		name:  name,
		limit: lim,
		burst: burst,
		byIP:  make(map[string]*ipLimiterEntry),
	}
}

func (l *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.byIP) > 2048 {
		cutoff := now.Add(-10 * time.Minute)
		for k, e := range l.byIP {
			if e.last.Before(cutoff) {
				delete(l.byIP, k)
			}
		}
	}

	e, ok := l.byIP[ip]
	if !ok {
		e = &ipLimiterEntry{lim: rate.NewLimiter(l.limit, l.burst)}
		l.byIP[ip] = e
	}
	e.last = now
	return e.lim
}

// Handler returns middleware that returns 429 when the client exceeds the limit.
func (l *IPRateLimiter) Handler(next http.Handler) http.Handler {
	if l == nil || next == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ClientIP(r)
		if !l.getLimiter(ip).Allow() {
			if l.log != nil {
				l.log.Warn("rate limit exceeded", "limiter", l.name, "client_ip", ip, "path", r.URL.Path)
			}
			response.WriteJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": map[string]any{
					"code":    "rate_limited",
					"message": "too many requests",
					"details": map[string]any{"limiter": l.name},
				},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
