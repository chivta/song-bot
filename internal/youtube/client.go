package youtube

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lrstanley/go-ytdlp"

	"github.com/arvlas/song-bot/internal/domain"
)

const (
	// searchPrefix asks yt-dlp's YouTube search extractor for n results.
	searchPrefix = "ytsearch%d:%s"
	// audioFormatSelector prefers an audio-only stream and only falls back to
	// stripping audio out of a video when none is offered.
	audioFormatSelector = "bestaudio/best"
	// progressInterval is how often yt-dlp's download progress is sampled. It
	// matches the pace at which Telegram tolerates message edits.
	progressInterval = 3 * time.Second
	// socketTimeout bounds a single stalled HTTP read inside yt-dlp.
	socketTimeout = 30
	// retries is how often yt-dlp retries a fragment before giving up.
	retries = "3"
)

// Client drives yt-dlp. Every method runs one short-lived subprocess, so calls
// are independent and safe to make concurrently.
type Client struct {
	bins    Binaries
	format  string
	quality string
	results int
}

// New builds a client over already installed binaries.
func New(bins Binaries, format, quality string, results int) *Client {
	return &Client{bins: bins, format: format, quality: quality, results: results}
}

// Search returns the top matches for a free-text query. It runs flat, so no
// per-video request is made — enough to choose between results, not to download.
func (c *Client) Search(ctx context.Context, query string) ([]domain.Candidate, error) {
	res, err := c.command().
		DumpJSON().
		FlatPlaylist().
		Run(ctx, fmt.Sprintf(searchPrefix, c.results, query))
	if err != nil {
		return nil, extractorError("search", err, res)
	}

	infos, err := res.GetExtractedInfo()
	if err != nil {
		return nil, fmt.Errorf("%w: parse search results: %v", domain.ErrExtractorFailed, err)
	}

	found := candidates(infos)
	if len(found) == 0 {
		return nil, domain.ErrNotFound
	}

	return found, nil
}

// Resolve fetches a single track's metadata without downloading anything, so
// duration and liveness can be checked before committing to a transfer.
func (c *Client) Resolve(ctx context.Context, url string) (domain.Song, error) {
	res, err := c.command().
		DumpJSON().
		NoPlaylist().
		Run(ctx, url)
	if err != nil {
		return domain.Song{}, extractorError("resolve", err, res)
	}

	infos, err := res.GetExtractedInfo()
	if err != nil {
		return domain.Song{}, fmt.Errorf("%w: parse metadata: %v", domain.ErrExtractorFailed, err)
	}
	if len(infos) == 0 || infos[0].ID == "" {
		return domain.Song{}, domain.ErrNotFound
	}

	return song(infos[0]), nil
}

// Download fetches a track's audio into dir, transcoding it to the configured
// format and writing cover art alongside it. Progress is reported through
// onProgress, which may be nil.
func (c *Client) Download(ctx context.Context, s domain.Song, dir string, onProgress func(domain.Progress)) (domain.Audio, error) {
	cmd := c.command().
		Output(filepath.Join(dir, "%(id)s.%(ext)s")).
		Format(audioFormatSelector).
		ExtractAudio().
		AudioFormat(c.format).
		AudioQuality(c.quality).
		// The cover is both embedded in the file's tags and kept on disk, the
		// latter for Telegram's separate thumbnail slot.
		WriteThumbnail().
		ConvertThumbnails("jpg").
		EmbedThumbnail().
		EmbedMetadata().
		NoPlaylist()

	if onProgress != nil {
		cmd = cmd.ProgressFunc(progressInterval, func(u ytdlp.ProgressUpdate) {
			onProgress(domain.Progress{Percent: u.Percent(), ETA: u.ETA()})
		})
	}

	res, err := cmd.Run(ctx, s.URL)
	if err != nil {
		return domain.Audio{}, extractorError("download", err, res)
	}

	audioPath, err := findOutput(dir, s.ID, c.format)
	if err != nil {
		return domain.Audio{}, err
	}

	info, err := os.Stat(audioPath)
	if err != nil {
		return domain.Audio{}, fmt.Errorf("stat downloaded audio: %w", err)
	}

	return domain.Audio{
		Song:  s,
		Path:  audioPath,
		Cover: squareCover(ctx, c.bins.FFmpeg, dir, s.ID),
		Size:  info.Size(),
	}, nil
}

// command builds a yt-dlp invocation with the flags every call shares.
func (c *Client) command() *ytdlp.Command {
	return ytdlp.New().
		SetExecutable(c.bins.YTDLP).
		NoWarnings().
		Retries(retries).
		SocketTimeout(socketTimeout)
}

// findOutput locates the produced audio file. The output template fixes the
// name, but post-processing picks the extension, so a glob is the fallback.
func findOutput(dir, id, format string) (string, error) {
	expected := filepath.Join(dir, id+"."+format)
	_, err := os.Stat(expected)
	if err == nil {
		return expected, nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, id+".*"))
	if err != nil {
		return "", fmt.Errorf("scan output directory: %w", err)
	}

	for _, m := range matches {
		if !strings.HasSuffix(m, ".jpg") && !strings.HasSuffix(m, ".webp") {
			return m, nil
		}
	}

	return "", fmt.Errorf("%w: yt-dlp produced no audio file", domain.ErrExtractorFailed)
}

// extractorError translates a failed yt-dlp run into a domain error. yt-dlp
// reports everything through its exit code, so the message is what tells a
// missing video apart from a broken extractor.
func extractorError(stage string, cause error, res *ytdlp.Result) error {
	stderr := ""
	if res != nil {
		stderr = res.Stderr
	}

	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "unsupported url"):
		return domain.ErrUnsupportedURL
	case strings.Contains(lower, "video unavailable"),
		strings.Contains(lower, "private video"),
		strings.Contains(lower, "does not exist"),
		strings.Contains(lower, "no video results"):
		return domain.ErrNotFound
	case errors.Is(cause, context.DeadlineExceeded), errors.Is(cause, context.Canceled):
		return cause
	}

	return fmt.Errorf("%w: %s: %v: %s", domain.ErrExtractorFailed, stage, cause, firstLine(stderr))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")

	return line
}
