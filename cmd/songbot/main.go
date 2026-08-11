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

// components is how many long-running goroutines run wants to collect from.
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// yt-dlp and ffmpeg are fetched before anything else: without them there is
	// nothing the bot can usefully answer.
	bins, err := youtube.Install(ctx, cfg.YTDLPDir, cfg.YTDLPPath)
	if err != nil {
		return err
	}

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	telegram, err := bot.New(cfg.BotToken, cfg.AllowedUsers)
	if err != nil {
		return fmt.Errorf("create telegram bot: %w", err)
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

	probes := health.New(cfg.HTTPAddr, metrics.Handler())

	var wg sync.WaitGroup
	errs := make(chan error, components)

	wg.Go(func() { errs <- pipeline.Run(ctx) })
	wg.Go(func() { errs <- telegram.Run(ctx, pipeline) })
	wg.Go(func() { errs <- probes.Run(ctx) })

	wg.Wait()
	close(errs)

	var joined error
	for err := range errs {
		joined = errors.Join(joined, err)
	}

	log.Info().Msg("shutdown complete")

	return joined
}
