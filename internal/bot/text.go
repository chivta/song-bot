package bot

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arvlas/song-bot/internal/domain"
)

// Every user-facing string lives here. Nothing in the other layers formats text
// for a human: they pass states and domain errors, and this file names them.
const (
	textStart = "🎵 Send me a song name or a YouTube link and I'll send back the audio.\n\n" +
		"For example:\n<code>bohemian rhapsody</code>\n<code>https://youtu.be/…</code>"
	textEmptyQuery = "Send me a song name or a YouTube link."
	textQueued     = "⏳ Queued…"
	textSearching  = "🔎 Searching…"
	textUploading  = "📤 Sending…"

	// textDownloading and textDownloadingPercent are the two forms of the
	// download state, before and after yt-dlp reports any progress.
	textDownloading        = "⬇️ <b>%s</b>\nStarting…"
	textDownloadingPercent = "⬇️ <b>%s</b>\n%.0f%%%s"
	textETA                = " · %s left"

	// textCaption is the caption under a delivered track.
	textCaption = "<a href=\"%s\">%s</a>"
)

// errorText maps every domain error onto the one sentence the user gets. An
// error missing from this map is a bug on our side, not theirs.
var errorText = map[error]string{
	domain.ErrNotFound:        "🤷 Couldn't find that one. Try adding the artist's name.",
	domain.ErrUnsupportedURL:  "🔗 I only understand YouTube links.",
	domain.ErrLiveStream:      "📡 That's a live stream — there's nothing to download yet.",
	domain.ErrTooLong:         "⏱ That's too long to send. Ask for a single track.",
	domain.ErrTooLarge:        "📦 The audio came out too big for Telegram's 50 MB limit.",
	domain.ErrRateLimited:     "🐢 You've hit your download limit. Try again in a little while.",
	domain.ErrQueueFull:       "🚦 I'm at capacity right now. Try again in a minute.",
	domain.ErrUnauthorized:    "🔒 This bot is private.",
	domain.ErrExtractorFailed: "💥 YouTube wouldn't hand that over. It may be blocked or region-locked.",
}

const textUnexpected = "💥 Something went wrong on my side. Try again."

// describe names an error for the user, falling back to the generic apology.
func describe(err error) string {
	for sentinel, text := range errorText {
		if errors.Is(err, sentinel) {
			return text
		}
	}

	return textUnexpected
}

// downloading renders the download state, showing an ETA only once yt-dlp has
// enough of the transfer behind it to guess one.
func downloading(title string, p domain.Progress) string {
	if p.Percent <= 0 {
		return fmt.Sprintf(textDownloading, escape(title))
	}

	eta := ""
	if p.ETA > 0 {
		eta = fmt.Sprintf(textETA, p.ETA.Round(time.Second))
	}

	return fmt.Sprintf(textDownloadingPercent, escape(title), p.Percent, eta)
}

// caption links a delivered track back to its source. Title and artist ride in
// the audio message's own fields, so the caption only carries the link.
func caption(s domain.Song) string {
	return fmt.Sprintf(textCaption, s.URL, escape(s.Title))
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
