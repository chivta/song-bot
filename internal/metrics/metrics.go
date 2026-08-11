package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

// Counters are process-wide, following the same "initialise once, call via
// package functions" rule as the logger.
var (
	requestsTotal    atomic.Int64
	requestsRejected atomic.Int64
	downloadsTotal   atomic.Int64
	downloadsFailed  atomic.Int64
	cacheHits        atomic.Int64
	queueDepth       atomic.Int64
	errorsTotal      atomic.Int64
)

// IncRequest records a song request accepted from a user.
func IncRequest() { requestsTotal.Add(1) }

// IncRejected records a request turned away by rate limiting or a full queue.
func IncRejected() { requestsRejected.Add(1) }

// IncDownload records a track downloaded and delivered.
func IncDownload() { downloadsTotal.Add(1) }

// IncDownloadFailed records a job that ended without a delivery.
func IncDownloadFailed() { downloadsFailed.Add(1) }

// IncCacheHit records a track re-sent from a stored Telegram file ID, which
// skips the download entirely.
func IncCacheHit() { cacheHits.Add(1) }

// SetQueueDepth publishes how many jobs are waiting for a worker.
func SetQueueDepth(n int) { queueDepth.Store(int64(n)) }

// IncErrors records an error-level event.
func IncErrors() { errorsTotal.Add(1) }

// Handler renders the counters in Prometheus text exposition format.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/plain; version=0.0.4")

		fmt.Fprintf(w, "# TYPE songbot_requests_total counter\nsongbot_requests_total %d\n", requestsTotal.Load())
		fmt.Fprintf(w, "# TYPE songbot_requests_rejected_total counter\nsongbot_requests_rejected_total %d\n", requestsRejected.Load())
		fmt.Fprintf(w, "# TYPE songbot_downloads_total counter\nsongbot_downloads_total %d\n", downloadsTotal.Load())
		fmt.Fprintf(w, "# TYPE songbot_downloads_failed_total counter\nsongbot_downloads_failed_total %d\n", downloadsFailed.Load())
		fmt.Fprintf(w, "# TYPE songbot_cache_hits_total counter\nsongbot_cache_hits_total %d\n", cacheHits.Load())
		fmt.Fprintf(w, "# TYPE songbot_queue_depth gauge\nsongbot_queue_depth %d\n", queueDepth.Load())
		fmt.Fprintf(w, "# TYPE songbot_errors_total counter\nsongbot_errors_total %d\n", errorsTotal.Load())
	})
}
