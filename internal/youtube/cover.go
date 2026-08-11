package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// thumbSize is Telegram's ceiling for a message thumbnail. Larger images
	// are ignored, and the slot expects a square.
	thumbSize = 320
	// thumbSuffix keeps the generated square apart from yt-dlp's own thumbnail.
	thumbSuffix = ".cover.jpg"
	// coverTimeout bounds the crop; it is a single small still image.
	coverTimeout = 15 * time.Second
)

// cropFilter scales the shorter side to thumbSize and centre-crops the rest,
// which turns YouTube's 16:9 still into album-art proportions.
var cropFilter = fmt.Sprintf("scale=%[1]d:%[1]d:force_original_aspect_ratio=increase,crop=%[1]d:%[1]d", thumbSize)

// squareCover crops yt-dlp's thumbnail into the square JPEG Telegram wants for
// the audio message. Cover art is a nicety, so any failure returns an empty
// path and is logged rather than failing the download.
func squareCover(ctx context.Context, ffmpeg, dir, id string) string {
	source := filepath.Join(dir, id+".jpg")
	_, err := os.Stat(source)
	if err != nil {
		log.Debug().Str("video_id", id).Msg("no thumbnail was written, sending without cover")
		return ""
	}

	dest := filepath.Join(dir, id+thumbSuffix)

	ctx, cancel := context.WithTimeout(ctx, coverTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, ffmpeg,
		"-hide_banner", "-loglevel", "error", "-y",
		"-i", source,
		"-vf", cropFilter,
		dest,
	).CombinedOutput()
	if err != nil {
		log.Warn().Err(err).Str("video_id", id).Str("ffmpeg", string(out)).Msg("failed to crop cover art")
		return ""
	}

	return dest
}
