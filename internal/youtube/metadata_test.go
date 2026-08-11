package youtube

import "testing"

func TestTitleAndArtist(t *testing.T) {
	tests := []struct {
		name       string
		track      string
		artist     string
		videoTitle string
		channel    string
		wantTitle  string
		wantArtist string
	}{
		{
			name:       "music metadata wins when yt-dlp supplies it",
			track:      "Around the World",
			artist:     "Daft Punk",
			videoTitle: "Daft Punk - Around the World (Official Video)",
			channel:    "Daft Punk",
			wantTitle:  "Around the World",
			wantArtist: "Daft Punk",
		},
		{
			name:       "artist - track title is split when metadata is absent",
			videoTitle: "Daft Punk - Around the World",
			wantTitle:  "Around the World",
			wantArtist: "Daft Punk",
		},
		{
			name:       "channel is trusted over the title's left half",
			videoTitle: "Daft Punk - Around the World",
			channel:    "Daft Punk Official",
			wantTitle:  "Around the World",
			wantArtist: "Daft Punk Official",
		},
		{
			name:       "promotional tags are stripped",
			videoTitle: "Around the World [Official Music Video] (HD)",
			channel:    "Daft Punk",
			wantTitle:  "Around the World",
			wantArtist: "Daft Punk",
		},
		{
			name:       "an unattributable upload still gets an artist",
			videoTitle: "some song",
			wantTitle:  "some song",
			wantArtist: unknownArtist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, artist := titleAndArtist(tt.track, tt.artist, tt.videoTitle, tt.channel)

			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if artist != tt.wantArtist {
				t.Errorf("artist = %q, want %q", artist, tt.wantArtist)
			}
		})
	}
}
