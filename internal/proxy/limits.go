package proxy

import "sync"

// connLimiter counts active proxied connections per server and enforces the
// per-server connection limit. Unlike tracked player sessions (which only
// start after a Login Start packet), it counts every TCP connection the
// server-listener Minecraft proxy routes to a backend, attributed to the
// server the connection was actually forwarded to (including fallback/lobby
// targets).
type connLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newConnLimiter() *connLimiter {
	return &connLimiter{counts: make(map[string]int)}
}

// acquire registers a new connection for serverID, enforcing the limit when
// limit > 0 (0 = unlimited). It reports whether the connection may proceed;
// on false the count is left unchanged. A nil limiter or empty serverID
// always allows the connection.
func (l *connLimiter) acquire(serverID string, limit int) bool {
	if l == nil || serverID == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if limit > 0 && l.counts[serverID] >= limit {
		return false
	}
	l.counts[serverID]++
	return true
}

// release drops one connection slot for serverID.
func (l *connLimiter) release(serverID string) {
	if l == nil || serverID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := l.counts[serverID]; n <= 1 {
		delete(l.counts, serverID)
	} else {
		l.counts[serverID] = n - 1
	}
}

// count returns the current number of active connections for serverID.
func (l *connLimiter) count(serverID string) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[serverID]
}
