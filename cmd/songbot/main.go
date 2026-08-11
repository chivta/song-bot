package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/arvlas/song-bot/internal/bot"
	"github.com/arvlas/song-bot/internal/config"
	"github.com/arvlas/song-bot/internal/downloader"
	"github.com/arvlas/song-bot/internal/health"
	"github.com/arvlas/song-bot/internal/logging"
	"github.com/arvlas/song-bot/internal/metrics"
	"github.com/arvlas/song-bot/internal/storage"
	"github.com/arvlas/song-bot/internal/youtube"
)

// components is how many long-running goroutines run collects errors from.
const components = 3

// mebibyte converts the configured size cap into bytes.
const mebibyte = 1 << 20

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logging.Init(cfg.LogLevel)

	err = run(cfg)
	if err != nil {
		log.Error().Err(err).Msg("songbot exited with an error")
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// A cancellable child of the signal context, so a failure during startup
	// can bring down whatever is already running.
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()

	var wg sync.WaitGroup
	errs := make(chan error, components)

	// The probe server goes up first and stays up for the whole run. Resolving
	// yt-dlp on a cold volume downloads it, which can outlast the liveness
	// probe's budget; answering /health throughout is what stops Kubernetes
	// from killing the pod mid-download.
	probes := health.New(cfg.HTTPAddr, metrics.Handler())
	wg.Go(func() { errs <- probes.Run(ctx) })

	// yt-dlp and ffmpeg come next: without them there is nothing the bot can
	// usefully answer.
	bins, err := youtube.Install(ctx, cfg.YTDLPDir, cfg.YTDLPPath)
	if err != nil {
		return shutdown(cancel, &wg, errs, err)
	}

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return shutdown(cancel, &wg, errs, err)
	}
	defer db.Close()

	telegram, err := bot.New(cfg.BotToken, cfg.AllowedUsers)
	if err != nil {
		return shutdown(cancel, &wg, errs, fmt.Errorf("create telegram bot: %w", err))
	}

	yt := youtube.New(bins, cfg.AudioFormat, cfg.AudioQuality, cfg.SearchResults)
	repo := storage.NewTrackRepo(db)

	// The bot and the pipeline are two halves of one loop: the bot submits jobs
	// and the pipeline delivers through the bot. The submitter is handed over at
	// Run rather than at construction so neither has to be built half-finished.
	pipeline := downloader.New(yt, repo, telegram, downloader.Limits{
		Workers:         cfg.Workers,
		QueueSize:       cfg.QueueSize,
		UserRatePerHour: cfg.UserRatePerHour,
		UserBurst:       cfg.UserBurst,
		MaxDuration:     cfg.MaxDuration,
		MaxFileSize:     cfg.MaxFileSizeMB * mebibyte,
		DownloadTimeout: cfg.DownloadTimeout,
		WorkDir:         cfg.WorkDir,
	})

	wg.Go(func() { errs <- pipeline.Run(ctx) })
	wg.Go(func() { errs <- telegram.Run(ctx, pipeline) })

	return shutdown(nil, &wg, errs, nil)
}

// shutdown waits for every started component to return and folds their errors
// together with cause. Passing a non-nil cancel stops them first, which is what
// a startup failure needs; a nil cancel means the components are already
// winding down on their own.
func shutdown(cancel context.CancelFunc, wg *sync.WaitGroup, errs chan error, cause error) error {
	if cancel != nil {
		cancel()
	}

	wg.Wait()
	close(errs)

	joined := cause
	for err := range errs {
		joined = errors.Join(joined, err)
	}

	log.Info().Msg("shutdown complete")

	return joined
}
