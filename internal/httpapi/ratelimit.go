package httpapi

import (
	"container/list"
	"sync"
	"time"
)

// Limiter is an in-memory sliding-window rate limiter keyed by client (IP
// or IPv6 /64). Nothing is persisted: a restart resets the counters, which
// is acceptable for a single API instance and keeps IP addresses out of the
// database.
//
// It holds at most MaxLimiterKeys clients, so memory stays bounded however
// many addresses come (IPv6 /64s, a botnet). A full limiter never refuses a
// new client for being full (that would let anyone close the service by
// filling it): it drops the clients whose hits have all expired and, if that
// is not enough, the least recently seen one. Every request counts as being
// seen, refused ones too, so a client that keeps trying stays limited however
// many other addresses rotate. Someone who rotates addresses themselves gets
// a fresh quota with each one: what holds them back are the global limits
// (per domain, per minute and per day) and the capped queue (docs/security.md).
type Limiter struct {
	mu      sync.Mutex
	windows []window
	clients map[string]*limiterClient
	lru     *list.List // of *limiterClient, most recently seen first
	now     func() time.Time
	swept   time.Time
	// fullSwept: when the last sweep forced by a full map ran (at most once
	// a second, so a flood of new clients does not cost a scan of the map each).
	fullSwept time.Time
}

type limiterClient struct {
	key  string
	hits []time.Time // oldest first, within the longest window
	elem *list.Element
}

// MaxLimiterKeys is how many clients one Limiter keeps (about 200 bytes
// each: some 20 MB when full). Security review of 01/11/2026.
const MaxLimiterKeys = 100_000

type window struct {
	span time.Duration
	max  int
}

// NewLimiter allows perMinute requests per minute and perDay per 24 hours.
func NewLimiter(perMinute, perDay int) *Limiter {
	return newLimiter(window{time.Minute, perMinute}, window{24 * time.Hour, perDay})
}

// NewWindowLimiter allows n events per span (e.g. 5 failed logins per
// 15 minutes).
func NewWindowLimiter(n int, span time.Duration) *Limiter {
	return newLimiter(window{span, n})
}

// newLimiter takes the windows shortest first.
func newLimiter(ws ...window) *Limiter {
	return &Limiter{windows: ws, clients: map[string]*limiterClient{}, lru: list.New(), now: time.Now}
}

// Reserve takes one slot for key. If a window is full it returns false and
// how long until a slot frees up.
func (l *Limiter) Reserve(key string) (ok bool, retryAfter time.Duration) {
	return l.take(key, true)
}

// Check reports whether key has a free slot without taking it. With Add it
// counts only some events, such as failed logins.
func (l *Limiter) Check(key string) (ok bool, retryAfter time.Duration) {
	return l.take(key, false)
}

// Add records one event for key, even if its windows are full.
func (l *Limiter) Add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	c := l.seen(key, now)
	c.hits = append(c.hits, now)
}

// Reset forgets key (e.g. its failed logins after a successful one).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.clients[key]; ok {
		l.forget(c)
	}
}

func (l *Limiter) take(key string, record bool) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	c := l.seen(key, now)
	c.hits = prune(c.hits, now.Add(-l.windows[len(l.windows)-1].span))
	for _, w := range l.windows {
		from := now.Add(-w.span)
		n, oldest := 0, time.Time{}
		for _, t := range c.hits {
			if t.After(from) {
				if n == 0 {
					oldest = t
				}
				n++
			}
		}
		if n >= w.max {
			return false, oldest.Add(w.span).Sub(now)
		}
	}
	if record {
		c.hits = append(c.hits, now)
	}
	return true, 0
}

// seen returns key's client, created if new, as the most recently seen. A
// new client in a full limiter first makes room: the clients with no hit
// left in the windows go, then the least recently seen ones.
func (l *Limiter) seen(key string, now time.Time) *limiterClient {
	if c, ok := l.clients[key]; ok {
		l.lru.MoveToFront(c.elem)
		return c
	}
	if len(l.clients) >= MaxLimiterKeys {
		if d := now.Sub(l.fullSwept); d >= time.Second || d < 0 {
			l.fullSwept = now
			l.swept = time.Time{}
			l.sweep(now)
		}
		for len(l.clients) >= MaxLimiterKeys {
			l.forget(l.lru.Back().Value.(*limiterClient))
		}
	}
	c := &limiterClient{key: key}
	c.elem = l.lru.PushFront(c)
	l.clients[key] = c
	return c
}

func (l *Limiter) forget(c *limiterClient) {
	l.lru.Remove(c.elem)
	delete(l.clients, c.key)
}

// Cancel gives back the last slot taken by key (e.g. the request reused an
// existing scan instead of starting one).
func (l *Limiter) Cancel(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if c, ok := l.clients[key]; ok && len(c.hits) > 0 {
		c.hits = c.hits[:len(c.hits)-1]
	}
}

// sweep drops idle keys every 10 minutes so memory stays bounded by recent clients.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.swept) < 10*time.Minute {
		return
	}
	l.swept = now
	cutoff := now.Add(-l.windows[len(l.windows)-1].span)
	for _, c := range l.clients {
		if c.hits = prune(c.hits, cutoff); len(c.hits) == 0 {
			l.forget(c)
		}
	}
}

func prune(h []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(h) && !h[i].After(cutoff) {
		i++
	}
	return h[i:]
}
