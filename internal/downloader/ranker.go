package downloader

import (
	"strings"
	"time"

	"github.com/arvlas/song-bot/internal/domain"
)

const (
	// rankWeight is how much YouTube's own ordering counts. It stays the
	// dominant signal — the heuristics below only break near-ties.
	rankWeight = 10
	// durationBonus rewards a runtime in the range an ordinary song occupies.
	durationBonus = 15
	// noisePenalty is charged per unwanted keyword found in a title.
	noisePenalty = 20
	// songMinLength and songMaxLength bracket a plausible single.
	songMinLength = 45 * time.Second
	songMaxLength = 10 * time.Minute
)

// noiseKeywords mark uploads that are usually not the recording someone asking
// for a song by name wants. A keyword is only held against a candidate when the
// user did not ask for it themselves.
var noiseKeywords = []string{
	"live", "cover", "karaoke", "instrumental", "remix", "reaction",
	"full album", "mix", "hour", "loop", "slowed", "nightcore", "tutorial",
}

// bestMatch picks the candidate most likely to be the track the user meant, and
// reports whether anything was usable at all.
func bestMatch(found []domain.Candidate, query string, maxDuration time.Duration) (domain.Candidate, bool) {
	lowerQuery := strings.ToLower(query)

	best := domain.Candidate{}
	bestScore := 0
	ok := false

	for i, c := range found {
		// A missing duration means a live stream or an unplayable upload.
		if c.Duration <= 0 || c.Duration > maxDuration {
			continue
		}

		score := (len(found) - i) * rankWeight
		if c.Duration >= songMinLength && c.Duration <= songMaxLength {
			score += durationBonus
		}
		score -= noisePenalty * unwantedKeywords(c.Title, lowerQuery)

		if !ok || score > bestScore {
			best, bestScore, ok = c, score, true
		}
	}

	return best, ok
}

// unwantedKeywords counts the noise keywords in a title that the user did not
// ask for, so a deliberate search for a live version is not punished.
func unwantedKeywords(title, lowerQuery string) int {
	lowerTitle := strings.ToLower(title)

	count := 0
	for _, kw := range noiseKeywords {
		if strings.Contains(lowerTitle, kw) && !strings.Contains(lowerQuery, kw) {
			count++
		}
	}

	return count
}
