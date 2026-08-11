package domain

import "time"

// Song is a single YouTube track, normalised from yt-dlp's metadata dump.
type Song struct {
	// ID is the YouTube video ID and the cache key for a delivered track.
	ID       string
	Title    string
	Artist   string
	Duration time.Duration
	// URL is the canonical watch page, shown to the user.
	URL string
	// ThumbnailURL is the best cover art yt-dlp knows about.
	ThumbnailURL string
	IsLive       bool
}

// Candidate is one entry of a search result. Searches run flat, so only the
// fields YouTube returns without a per-video request are populated.
type Candidate struct {
	ID       string
	Title    string
	Uploader string
	Duration time.Duration
}

// Audio is a downloaded track on local disk, ready to be delivered.
type Audio struct {
	Song Song
	// Path is the audio file. Cover is a square JPEG thumbnail, empty when the
	// cover art could not be produced.
	Path  string
	Cover string
	Size  int64
}

// Job is one user request travelling through the download queue.
type Job struct {
	// UserID and ChatID identify who asked and where the answer goes.
	UserID int64
	ChatID int64
	// StatusMessageID is the placeholder message edited with progress.
	StatusMessageID int
	// Query is either a YouTube URL or free text to search for.
	Query string
	// IsURL records which of the two Query is, decided at the delivery layer.
	IsURL       bool
	RequestedAt time.Time
}

// Progress is a point-in-time download state reported back to the user.
type Progress struct {
	Percent float64
	ETA     time.Duration
}
