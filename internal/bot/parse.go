package bot

import (
	"net/url"
	"strings"

	"github.com/arvlas/song-bot/internal/domain"
)

// httpsScheme is prepended to bare hostnames so "youtu.be/…" parses as a URL.
const httpsScheme = "https://"

// youtubeHosts are the domains a watchable link can arrive on.
var youtubeHosts = map[string]bool{
	"youtube.com":       true,
	"www.youtube.com":   true,
	"m.youtube.com":     true,
	"music.youtube.com": true,
	"youtu.be":          true,
	"www.youtu.be":      true,
}

// classify decides whether the user sent a link or a search query. Links to
// anywhere other than YouTube are rejected rather than searched for, since
// searching for a URL never produces what the user wanted.
func classify(text string) (string, bool, error) {
	trimmed := strings.TrimSpace(text)

	if !looksLikeURL(trimmed) {
		return trimmed, false, nil
	}

	withScheme := trimmed
	if !strings.Contains(withScheme, "://") {
		withScheme = httpsScheme + withScheme
	}

	parsed, err := url.Parse(withScheme)
	if err != nil || !youtubeHosts[strings.ToLower(parsed.Host)] {
		return "", true, domain.ErrUnsupportedURL
	}

	return withScheme, true, nil
}

// looksLikeURL is deliberately loose: anything that could be a link is handed
// to the parser, which has the final say.
func looksLikeURL(text string) bool {
	if strings.Contains(text, " ") {
		return false
	}

	lower := strings.ToLower(text)

	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "youtu.be/") ||
		strings.HasPrefix(lower, "youtube.com/") ||
		strings.HasPrefix(lower, "www.youtube.com/") ||
		strings.HasPrefix(lower, "m.youtube.com/") ||
		strings.HasPrefix(lower, "music.youtube.com/")
}
