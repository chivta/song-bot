package youtube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lrstanley/go-ytdlp"
	"github.com/rs/zerolog/log"
)

// Binaries are the external executables the client drives. They are downloaded
// and checksum-verified by go-ytdlp rather than baked into the image, so a
// yt-dlp bump does not require an app release.
type Binaries struct {
	YTDLP  string
	FFmpeg string
}

// Install resolves yt-dlp, ffmpeg and ffprobe into dir, downloading whatever is
// missing. Passing ytdlpPath skips the managed yt-dlp and uses that executable
// instead, which is how an operator runs a newer yt-dlp than this build pins.
func Install(ctx context.Context, dir, ytdlpPath string) (Binaries, error) {
	err := useCacheDir(dir)
	if err != nil {
		return Binaries{}, err
	}

	ffmpeg, err := ytdlp.InstallFFmpeg(ctx, nil)
	if err != nil {
		return Binaries{}, fmt.Errorf("install ffmpeg: %w", err)
	}

	_, err = ytdlp.InstallFFprobe(ctx, nil)
	if err != nil {
		return Binaries{}, fmt.Errorf("install ffprobe: %w", err)
	}

	bins := Binaries{YTDLP: ytdlpPath, FFmpeg: ffmpeg.Executable}

	if bins.YTDLP == "" {
		resolved, err := ytdlp.Install(ctx, nil)
		if err != nil {
			return Binaries{}, fmt.Errorf("install yt-dlp: %w", err)
		}
		bins.YTDLP = resolved.Executable

		log.Info().
			Str("path", resolved.Executable).
			Str("version", resolved.Version).
			Bool("downloaded", resolved.Downloaded).
			Msg("yt-dlp ready")
	} else {
		log.Info().Str("path", bins.YTDLP).Msg("using operator-supplied yt-dlp")
	}

	log.Info().Str("ffmpeg", bins.FFmpeg).Msg("ffmpeg ready")

	return bins, nil
}

// useCacheDir points go-ytdlp's binary cache at dir. go-ytdlp derives it from
// the XDG cache directory, so the variable is set for the whole process before
// any install runs — the one piece of global state this package owns.
func useCacheDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve binary directory: %w", err)
	}

	err = os.MkdirAll(abs, 0o750)
	if err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}

	err = os.Setenv("XDG_CACHE_HOME", abs)
	if err != nil {
		return fmt.Errorf("set binary cache directory: %w", err)
	}

	return nil
}
