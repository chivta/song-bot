package downloader

import (
	"testing"
	"time"

	"github.com/arvlas/song-bot/internal/domain"
)

const maxDuration = 20 * time.Minute

func TestBestMatchPrefersYouTubesOwnOrdering(t *testing.T) {
	found := []domain.Candidate{
		{ID: "first", Title: "Song", Duration: 3 * time.Minute},
		{ID: "second", Title: "Song", Duration: 3 * time.Minute},
	}

	match, ok := bestMatch(found, "song", maxDuration)
	if !ok || match.ID != "first" {
		t.Fatalf("match = %+v, ok = %v, want the first result", match, ok)
	}
}

func TestBestMatchSkipsLiveAndOverlongEntries(t *testing.T) {
	found := []domain.Candidate{
		// A missing duration is what a live stream looks like in flat results.
		{ID: "live", Title: "Song", Duration: 0},
		{ID: "marathon", Title: "Song", Duration: 3 * time.Hour},
		{ID: "single", Title: "Song", Duration: 4 * time.Minute},
	}

	match, ok := bestMatch(found, "song", maxDuration)
	if !ok || match.ID != "single" {
		t.Fatalf("match = %+v, ok = %v, want the playable single", match, ok)
	}
}

func TestBestMatchDemotesNoiseTheUserDidNotAskFor(t *testing.T) {
	found := []domain.Candidate{
		{ID: "live-version", Title: "Song (Live at Wembley)", Duration: 4 * time.Minute},
		{ID: "studio", Title: "Song", Duration: 4 * time.Minute},
	}

	match, ok := bestMatch(found, "song", maxDuration)
	if !ok || match.ID != "studio" {
		t.Fatalf("match = %+v, ok = %v, want the studio recording", match, ok)
	}
}

func TestBestMatchHonoursNoiseTermsInTheQuery(t *testing.T) {
	found := []domain.Candidate{
		{ID: "live-version", Title: "Song (Live at Wembley)", Duration: 4 * time.Minute},
		{ID: "studio", Title: "Song", Duration: 4 * time.Minute},
	}

	// Asking for a live version must not be penalised for being one.
	match, ok := bestMatch(found, "song live", maxDuration)
	if !ok || match.ID != "live-version" {
		t.Fatalf("match = %+v, ok = %v, want the live recording", match, ok)
	}
}

func TestBestMatchReportsWhenNothingIsUsable(t *testing.T) {
	found := []domain.Candidate{{ID: "stream", Title: "24/7 radio", Duration: 0}}

	_, ok := bestMatch(found, "radio", maxDuration)
	if ok {
		t.Fatal("ok = true, want false when every candidate is unplayable")
	}
}
