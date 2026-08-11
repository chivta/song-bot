package bot

import (
	"errors"
	"testing"

	"github.com/arvlas/song-bot/internal/domain"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantQuery string
		wantURL   bool
		wantErr   error
	}{
		{
			name:      "plain query",
			input:     "  daft punk around the world  ",
			wantQuery: "daft punk around the world",
		},
		{
			name:      "watch url",
			input:     "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantQuery: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
			wantURL:   true,
		},
		{
			name:      "short url without scheme gets one",
			input:     "youtu.be/dQw4w9WgXcQ",
			wantQuery: "https://youtu.be/dQw4w9WgXcQ",
			wantURL:   true,
		},
		{
			name:      "youtube music url",
			input:     "https://music.youtube.com/watch?v=dQw4w9WgXcQ",
			wantQuery: "https://music.youtube.com/watch?v=dQw4w9WgXcQ",
			wantURL:   true,
		},
		{
			name:    "non-youtube link is rejected, not searched for",
			input:   "https://soundcloud.com/artist/track",
			wantURL: true,
			wantErr: domain.ErrUnsupportedURL,
		},
		{
			// A title containing a dot must not be mistaken for a hostname.
			name:      "query with a dot stays a query",
			input:     "r.e.m. losing my religion",
			wantQuery: "r.e.m. losing my religion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, isURL, err := classify(tt.input)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if isURL != tt.wantURL {
				t.Errorf("isURL = %v, want %v", isURL, tt.wantURL)
			}
			if query != tt.wantQuery {
				t.Errorf("query = %q, want %q", query, tt.wantQuery)
			}
		})
	}
}

func TestDescribeFallsBackToGenericText(t *testing.T) {
	if got := describe(domain.ErrTooLong); got != errorText[domain.ErrTooLong] {
		t.Errorf("describe(ErrTooLong) = %q, want the mapped text", got)
	}

	if got := describe(errors.New("some internal failure")); got != textUnexpected {
		t.Errorf("describe(unknown) = %q, want %q", got, textUnexpected)
	}
}

func TestDescribeUnwrapsWrappedSentinels(t *testing.T) {
	wrapped := errors.Join(errors.New("yt-dlp exit 1"), domain.ErrExtractorFailed)

	if got := describe(wrapped); got != errorText[domain.ErrExtractorFailed] {
		t.Errorf("describe(wrapped) = %q, want the extractor text", got)
	}
}
