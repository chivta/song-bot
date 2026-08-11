package downloader

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// idleTTL is how long a user's bucket is kept after their last request. It only
// bounds memory: a bucket discarded while full is indistinguishable from a new
// one, and a bucket is only full once the user has been quiet for an hour.
const idleTTL = 2 * time.Hour

// userLimiter gives every user their own token bucket, so one person queueing a
// playlist's worth of songs cannot spend everybody else's budget.
type userLimiter struct {
	mu      sync.Mutex
	buckets map[int64]*bucket
	limit   rate.Limit
	burst   int
}

type bucket struct {
	limiter *rate.Limiter
	lastUse time.Time
}

// newUserLimiter builds a limiter allowing perHour requests per user, with
// burst of them available at once.
func newUserLimiter(perHour, burst int) *userLimiter {
	return &userLimiter{
		buckets: make(map[int64]*bucket),
		limit:   rate.Every(time.Hour / time.Duration(perHour)),
		burst:   burst,
	}
}

// allow consumes one token for a user, reporting whether they had one.
func (l *userLimiter) allow(userID int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[userID]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[userID] = b
	}
	b.lastUse = time.Now()

	return b.limiter.Allow()
}

// evictIdle drops buckets nobody has touched recently.
func (l *userLimiter) evictIdle() {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-idleTTL)
	for id, b := range l.buckets {
		if b.lastUse.Before(cutoff) {
			delete(l.buckets, id)
		}
	}
}
