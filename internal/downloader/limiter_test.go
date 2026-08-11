package downloader

import "testing"

func TestUserLimiterSpendsOnlyTheCallersBudget(t *testing.T) {
	const burst = 3
	limiter := newUserLimiter(1, burst)

	for i := range burst {
		if !limiter.allow(1) {
			t.Fatalf("request %d was denied inside the burst", i+1)
		}
	}

	if limiter.allow(1) {
		t.Error("a fourth request was allowed past the burst")
	}

	// A user who has spent nothing must be unaffected by one who has.
	if !limiter.allow(2) {
		t.Error("a different user was denied their first request")
	}
}

func TestUserLimiterEvictsIdleBuckets(t *testing.T) {
	limiter := newUserLimiter(1, 1)
	limiter.allow(1)

	limiter.evictIdle()
	if len(limiter.buckets) != 1 {
		t.Fatalf("buckets = %d, want the recently used one kept", len(limiter.buckets))
	}

	limiter.buckets[1].lastUse = limiter.buckets[1].lastUse.Add(-2 * idleTTL)
	limiter.evictIdle()

	if len(limiter.buckets) != 0 {
		t.Errorf("buckets = %d, want the idle one dropped", len(limiter.buckets))
	}
}
