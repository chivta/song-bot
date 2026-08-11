package youtube

import (
	"regexp"
	"strings"
	"time"

	"github.com/lrstanley/go-ytdlp"

	"github.com/arvlas/song-bot/internal/domain"
)

const (
	// watchURL renders a canonical watch page from a video ID.
	watchURL = "https://www.youtube.com/watch?v="
	// topicSuffix marks YouTube Music's auto-generated artist channels, whose
	// uploads carry proper track metadata and clean audio.
	topicSuffix = " - Topic"
	// titleSeparator is the convention uploaders use for "Artist - Track".
	titleSeparator = " - "
	// unknownArtist is shown when nothing better can be derived.
	unknownArtist = "Unknown artist"
)

// noiseSuffixes are the promotional tags uploaders append to song titles. They
// are stripped so the delivered track reads like a library entry.
var noiseSuffixes = regexp.MustCompile(`(?i)\s*[\(\[][^)\]]*(official|lyric|audio|video|hd|hq|4k|visuali[sz]er|mv)[^)\]]*[\)\]]`)

// song maps a yt-dlp metadata dump onto the domain type.
func song(info *ytdlp.ExtractedInfo) domain.Song {
	s := domain.Song{
		ID:           info.ID,
		URL:          watchURL + info.ID,
		ThumbnailURL: bestThumbnail(info),
		IsLive:       deref(info.IsLive),
	}

	if info.WebpageURL != nil && *info.WebpageURL != "" {
		s.URL = *info.WebpageURL
	}
	if info.Duration != nil {
		s.Duration = time.Duration(*info.Duration * float64(time.Second))
	}

	s.Title, s.Artist = titleAndArtist(
		deref(info.Track),
		deref(info.Artist),
		deref(info.Title),
		channelName(info),
	)

	return s
}

// candidates maps flat search entries onto the domain type. Flat entries carry
// only what YouTube returns without a per-video request.
func candidates(entries []*ytdlp.ExtractedInfo) []domain.Candidate {
	out := make([]domain.Candidate, 0, len(entries))

	for _, e := range entries {
		if e == nil || e.ID == "" {
			continue
		}

		c := domain.Candidate{
			ID:       e.ID,
			Title:    deref(e.Title),
			Uploader: channelName(e),
		}
		if e.Duration != nil {
			c.Duration = time.Duration(*e.Duration * float64(time.Second))
		}

		out = append(out, c)
	}

	return out
}

// titleAndArtist prefers yt-dlp's music metadata, which YouTube Music supplies
// and ordinary uploads do not. It falls back to splitting the video title on
// the "Artist - Track" convention, and finally to the channel name.
func titleAndArtist(track, artist, videoTitle, channel string) (string, string) {
	if track != "" && artist != "" {
		return track, artist
	}

	clean := strings.TrimSpace(noiseSuffixes.ReplaceAllString(videoTitle, ""))
	if clean == "" {
		clean = strings.TrimSpace(videoTitle)
	}

	if artist == "" {
		artist = channel
	}

	if track == "" {
		before, after, found := strings.Cut(clean, titleSeparator)
		if found && strings.TrimSpace(before) != "" && strings.TrimSpace(after) != "" {
			// The channel already told us the artist, so the split only
			// contributes the track name.
			if artist != "" {
				return strings.TrimSpace(after), artist
			}
			return strings.TrimSpace(after), strings.TrimSpace(before)
		}
		track = clean
	}

	if artist == "" {
		artist = unknownArtist
	}

	return track, artist
}

// channelName resolves the uploading channel, dropping the "- Topic" suffix
// YouTube Music appends to auto-generated artist channels.
func channelName(info *ytdlp.ExtractedInfo) string {
	name := deref(info.Channel)
	if name == "" {
		name = deref(info.Uploader)
	}

	return strings.TrimSpace(strings.TrimSuffix(name, topicSuffix))
}

// bestThumbnail picks the largest cover art yt-dlp listed, falling back to the
// single thumbnail field when the list is absent.
func bestThumbnail(info *ytdlp.ExtractedInfo) string {
	best := ""
	bestArea := 0

	for _, t := range info.Thumbnails {
		if t == nil || t.URL == "" || t.Width == nil || t.Height == nil {
			continue
		}
		area := *t.Width * *t.Height
		if area > bestArea {
			best, bestArea = t.URL, area
		}
	}

	if best == "" {
		best = deref(info.Thumbnail)
	}

	return best
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}

	return *p
}
