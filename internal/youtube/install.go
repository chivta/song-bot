package youtube

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/lrstanley/go-ytdlp"
	"github.com/rs/zerolog/log"
)

const (
	// ffmpegBinary and ffprobeBinary are the companions yt-dlp shells out to
	// for audio extraction and tagging.
	ffmpegBinary  = "ffmpeg"
	ffprobeBinary = "ffprobe"
	// cacheSubdir is the directory go-ytdlp keeps managed binaries in, below
	// the XDG cache root.
	cacheSubdir = "go-ytdlp"
	// versionTimeout bounds the probe that checks a binary can actually run.
	versionTimeout = 10 * time.Second
	// installAttempts and installBackoff govern retrying the yt-dlp download.
	installAttempts = 4
	installBackoff  = 5 * time.Second
)

// Binaries are the external executables the client drives.
type Binaries struct {
	YTDLP  string
	FFmpeg string
}

// Install resolves yt-dlp, ffmpeg and ffprobe, downloading whatever is missing.
// Passing ytdlpPath skips the managed yt-dlp and uses that executable instead,
// which is how an operator runs a newer yt-dlp than this build pins.
func Install(ctx context.Context, dir, ytdlpPath string) (Binaries, error) {
	cacheDir, err := useCacheDir(dir)
	if err != nil {
		return Binaries{}, err
	}

	// A binary that cannot run poisons everything downstream, so clear it out
	// before anything tries to resolve it. See purgeUnusable.
	purgeUnusable(ctx, cacheDir)

	ffmpeg, err := resolveFFmpeg(ctx)
	if err != nil {
		return Binaries{}, err
	}

	bins := Binaries{YTDLP: ytdlpPath, FFmpeg: ffmpeg}

	if bins.YTDLP == "" {
		resolved, err := installYTDLP(ctx)
		if err != nil {
			return Binaries{}, err
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

	return bins, nil
}

// installYTDLP resolves yt-dlp, retrying a download that did not finish in
// time. go-ytdlp caps each attempt with its own 30s HTTP timeout, which a cold
// volume on a slow link can exceed; retrying in process turns what would
// otherwise be a crash loop into a slower but successful start.
func installYTDLP(ctx context.Context) (*ytdlp.ResolvedInstall, error) {
	var err error

	for attempt := 1; attempt <= installAttempts; attempt++ {
		var resolved *ytdlp.ResolvedInstall

		resolved, err = ytdlp.Install(ctx, nil)
		if err == nil {
			return resolved, nil
		}
		if ctx.Err() != nil {
			break
		}

		log.Warn().Err(err).Int("attempt", attempt).Msg("yt-dlp install failed, retrying")
		sleep(ctx, installBackoff)
	}

	return nil, fmt.Errorf("install yt-dlp: %w", err)
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// resolveFFmpeg prefers whatever is on PATH. In the container that is the
// distribution's own musl build, and it is the only one that runs there:
// go-ytdlp downloads glibc-linked ffmpeg builds, for which no musl variant is
// published, and those fail to exec on Alpine. The managed download is kept
// only as the fallback for a developer machine without ffmpeg installed.
func resolveFFmpeg(ctx context.Context) (string, error) {
	ffmpeg, ffmpegErr := lookupUsable(ctx, ffmpegBinary)
	_, ffprobeErr := lookupUsable(ctx, ffprobeBinary)

	if ffmpegErr == nil && ffprobeErr == nil {
		log.Info().Str("path", ffmpeg).Msg("using system ffmpeg")
		return ffmpeg, nil
	}

	log.Info().
		AnErr("ffmpeg", ffmpegErr).
		AnErr("ffprobe", ffprobeErr).
		Msg("no usable ffmpeg on PATH, falling back to the managed download")

	resolved, err := ytdlp.InstallFFmpeg(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("install ffmpeg: %w", err)
	}

	_, err = ytdlp.InstallFFprobe(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("install ffprobe: %w", err)
	}

	return resolved.Executable, nil
}

// lookupUsable finds a binary on PATH and confirms it actually executes.
// Presence is not enough: a binary built against the wrong libc is there, is
// executable, and still fails with "not found" the moment it is run.
func lookupUsable(ctx context.Context, name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("look up %s: %w", name, err)
	}

	err = runnable(ctx, path)
	if err != nil {
		return "", err
	}

	return path, nil
}

// purgeUnusable deletes managed ffmpeg and ffprobe binaries that cannot run.
// They are worth removing rather than ignoring for two reasons: go-ytdlp
// resolves its cache ahead of PATH and hard-fails when the cached binary will
// not execute, and it prepends the cache directory to PATH when it runs yt-dlp,
// where a broken binary would shadow the working system one during
// post-processing.
func purgeUnusable(ctx context.Context, cacheDir string) {
	for _, name := range []string{ffmpegBinary, ffprobeBinary} {
		path := filepath.Join(cacheDir, name)

		_, err := os.Stat(path)
		if err != nil {
			continue
		}

		err = runnable(ctx, path)
		if err == nil {
			continue
		}

		rmErr := os.Remove(path)
		if rmErr != nil {
			log.Error().Err(rmErr).Str("path", path).Msg("failed to remove unusable cached binary")
			continue
		}

		log.Warn().Err(err).Str("path", path).Msg("removed cached binary that cannot run here")
	}
}

// runnable reports whether an executable can be started at all.
func runnable(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()

	err := exec.CommandContext(ctx, path, "-version").Run()
	if err != nil {
		return fmt.Errorf("run %s: %w", path, err)
	}

	return nil
}

// useCacheDir points go-ytdlp's binary cache at dir and returns the directory
// the binaries themselves land in. go-ytdlp derives the location from the XDG
// cache directory, so the variable is set for the whole process before any
// install runs — the one piece of global state this package owns.
func useCacheDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve binary directory: %w", err)
	}

	err = os.MkdirAll(abs, 0o750)
	if err != nil {
		return "", fmt.Errorf("create binary directory: %w", err)
	}

	err = os.Setenv("XDG_CACHE_HOME", abs)
	if err != nil {
		return "", fmt.Errorf("set binary cache directory: %w", err)
	}

	return filepath.Join(abs, cacheSubdir), nil
}
