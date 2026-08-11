package domain

import "errors"

// Sentinel errors describing every condition the bot knows how to explain to a
// user. Anything else surfaces as a generic failure.
var (
	// ErrNotFound is returned when a search yields nothing usable.
	ErrNotFound = errors.New("not_found")
	// ErrUnsupportedURL is returned for links that are not YouTube watch pages.
	ErrUnsupportedURL = errors.New("unsupported_url")
	// ErrLiveStream is returned for live broadcasts, which have no fixed length.
	ErrLiveStream = errors.New("live_stream")
	// ErrTooLong is returned when a track exceeds the configured duration cap.
	ErrTooLong = errors.New("too_long")
	// ErrTooLarge is returned when the finished file exceeds Telegram's upload limit.
	ErrTooLarge = errors.New("too_large")
	// ErrRateLimited is returned when a user has spent their request budget.
	ErrRateLimited = errors.New("rate_limited")
	// ErrQueueFull is returned when the download queue cannot accept more work.
	ErrQueueFull = errors.New("queue_full")
	// ErrUnauthorized is returned when a user is not on the allow list.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrExtractorFailed is returned when yt-dlp itself fails, usually because
	// YouTube changed something and the pinned version needs a bump.
	ErrExtractorFailed = errors.New("extractor_failed")
)
