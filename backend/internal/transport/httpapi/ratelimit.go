package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipRateLimiter is a token bucket per client IP.
//
// The IP is taken from the TCP connection. Behind a reverse proxy it would come
// from X-Forwarded-For, but only from trusted proxies; trusting that header
// blindly would let anyone bypass the limit by spoofing it.
type ipRateLimiter struct {
	mu        sync.Mutex
	limit     rate.Limit
	burst     int
	visitors  map[string]*visitor
	lastSweep time.Time
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

const visitorTTL = 10 * time.Minute

func newIPRateLimiter(limit rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{limit: limit, burst: burst, visitors: make(map[string]*visitor), lastSweep: time.Now()}
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.Sub(l.lastSweep) > visitorTTL {
		for k, v := range l.visitors {
			if now.Sub(v.lastSeen) > visitorTTL {
				delete(l.visitors, k)
			}
		}
		l.lastSweep = now
	}
	v, ok := l.visitors[ip]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.visitors[ip] = v
	}
	v.lastSeen = now
	return v.limiter.Allow()
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !l.allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": map[string]string{"code": "rate_limited", "message": "too many requests"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}
