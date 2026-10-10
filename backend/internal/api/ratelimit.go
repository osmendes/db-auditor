package api

import (
	"net"
	"sync"
	"time"
)

// attemptWindow counts requests inside a one-minute bucket.
type attemptWindow struct {
	count int
	start time.Time
}

// apiWindow limits requests inside one process. It is not the login limiter:
// login attempts are stored in the snapshot store so a restart or another
// replica still blocks. Authenticated API calls use 600 per user per minute.
// Public mutations that create audit runs or reports use 20 per client IP per minute.
type apiWindow struct {
	mu    sync.Mutex
	byKey map[string]attemptWindow
	max   int
}

func (l *apiWindow) allow(key string) bool {
	host, _, err := net.SplitHostPort(key)
	if err != nil {
		host = key
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.byKey) > 10000 {
		for k, v := range l.byKey {
			if now.Sub(v.start) > time.Minute {
				delete(l.byKey, k)
			}
		}
	}
	v := l.byKey[host]
	if now.Sub(v.start) >= time.Minute {
		v = attemptWindow{start: now}
	}
	v.count++
	l.byKey[host] = v
	limit := l.max
	if limit <= 0 {
		limit = 10
	}
	return v.count <= limit
}
