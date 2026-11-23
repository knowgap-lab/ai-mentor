package auth

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ipLimit помнит моменты запросов /auth/* и /invite/* с одного IP (AU-R20).
type ipLimit struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (s *Service) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/auth/") || strings.HasPrefix(path, "/invite/") {
			if retry, ok := s.hits.allow(clientIP(r), time.Now()); !ok {
				w.Header().Set("Retry-After", strconv.Itoa(retry))
				writeErr(w, errRateLimited())
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (l *ipLimit) allow(ip string, now time.Time) (int, bool) {
	if l == nil {
		return 0, true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-time.Minute)
	kept := l.hits[ip][:0]
	for _, ts := range l.hits[ip] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= 10 {
		l.hits[ip] = kept
		retry := int(kept[0].Add(time.Minute).Sub(now).Seconds())
		if retry < 1 {
			retry = 1
		}
		return retry, false
	}
	l.hits[ip] = append(kept, now)
	return 0, true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return r.RemoteAddr
	}
	return host
}
