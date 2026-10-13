package httpapi

import (
	"sync"
	"time"
)

// Limiter is an in-memory sliding-window rate limiter keyed by client (IP
// or IPv6 /64). Nothing is persisted: a restart resets the counters, which
// is acceptable for a single API instance and keeps IP addresses out of the
// database.
type Limiter struct {
	mu      sync.Mutex
	windows []window
	hits    map[string][]time.Time // per key, oldest first, within the longest window
	now     func() time.Time
	swept   time.Time
}

type window struct {
	span time.Duration
	max  int
}

// NewLimiter allows perMinute requests per minute and perDay per 24 hours.
func NewLimiter(perMinute, perDay int) *Limiter {
	return &Limiter{
		windows: []window{{time.Minute, perMinute}, {24 * time.Hour, perDay}},
		hits:    map[string][]time.Time{},
		now:     time.Now,
	}
}

// Reserve takes one slot for key. If a window is full it returns false and
// how long until a slot frees up.
func (l *Limiter) Reserve(key string) (ok bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	longest := l.windows[len(l.windows)-1].span
	h := prune(l.hits[key], now.Add(-longest))
	for _, w := range l.windows {
		from := now.Add(-w.span)
		n, oldest := 0, time.Time{}
		for _, t := range h {
			if t.After(from) {
				if n == 0 {
					oldest = t
				}
				n++
			}
		}
		if n >= w.max {
			l.hits[key] = h
			return false, oldest.Add(w.span).Sub(now)
		}
	}
	l.hits[key] = append(h, now)
	return true, 0
}

// Cancel gives back the last slot taken by key (e.g. the request reused an
// existing scan instead of starting one).
func (l *Limiter) Cancel(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if h := l.hits[key]; len(h) > 0 {
		l.hits[key] = h[:len(h)-1]
	}
}

// sweep drops idle keys every 10 minutes so memory stays bounded by recent clients.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.swept) < 10*time.Minute {
		return
	}
	l.swept = now
	cutoff := now.Add(-l.windows[len(l.windows)-1].span)
	for k, h := range l.hits {
		if h = prune(h, cutoff); len(h) == 0 {
			delete(l.hits, k)
		} else {
			l.hits[k] = h
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
