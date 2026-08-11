package downloader

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/arvlas/song-bot/internal/domain"
	"github.com/arvlas/song-bot/internal/metrics"
)

const (
	// evictInterval is how often idle rate-limit buckets are swept.
	evictInterval = 30 * time.Minute
	// progressStep is the smallest change in percent worth telling the user
	// about. Together with progressMinGap it keeps message edits well under
	// Telegram's per-chat limit.
	progressStep = 10
	// progressMinGap is the minimum time between two progress updates.
	progressMinGap = 4 * time.Second
	// notifyTimeout bounds telling a user their job failed.
	notifyTimeout = 10 * time.Second
)

type youtubeClient interface {
	Search(ctx context.Context, query string) ([]domain.Candidate, error)
	Resolve(ctx context.Context, url string) (domain.Song, error)
	Download(ctx context.Context, s domain.Song, dir string, onProgress func(domain.Progress)) (domain.Audio, error)
}

// trackRepo remembers the Telegram file ID of every delivered track, so a song
// someone already asked for is re-sent without touching YouTube at all.
type trackRepo interface {
	FileID(ctx context.Context, videoID string) (string, error)
	Save(ctx context.Context, s domain.Song, fileID string) error
}

// deliverer is the delivery layer seen from here. It takes states, never
// prepared text: every user-facing string belongs to the bot package.
type deliverer interface {
	Searching(ctx context.Context, job domain.Job) error
	Downloading(ctx context.Context, job domain.Job, s domain.Song, p domain.Progress) error
	Uploading(ctx context.Context, job domain.Job, s domain.Song) error
	Deliver(ctx context.Context, job domain.Job, a domain.Audio) (string, error)
	DeliverCached(ctx context.Context, job domain.Job, s domain.Song, fileID string) error
	Fail(ctx context.Context, job domain.Job, cause error)
}

// Limits are the policy knobs the service enforces on every job.
type Limits struct {
	Workers         int
	QueueSize       int
	UserRatePerHour int
	UserBurst       int
	MaxDuration     time.Duration
	MaxFileSize     int64
	DownloadTimeout time.Duration
	WorkDir         string
}

// Service is the download pipeline: a bounded queue drained by a fixed pool of
// workers. Requests are accepted and answered asynchronously, so a slow
// download never holds up another user's request.
type Service struct {
	yt     youtubeClient
	repo   trackRepo
	out    deliverer
	users  *userLimiter
	limits Limits
	jobs   chan domain.Job
}

// New builds the pipeline. Nothing runs until Run is called.
func New(yt youtubeClient, repo trackRepo, out deliverer, limits Limits) *Service {
	return &Service{
		yt:     yt,
		repo:   repo,
		out:    out,
		users:  newUserLimiter(limits.UserRatePerHour, limits.UserBurst),
		limits: limits,
		jobs:   make(chan domain.Job, limits.QueueSize),
	}
}

// Submit accepts a request for processing. It never blocks: a user over their
// budget or a full queue is rejected immediately so the caller can say so.
func (s *Service) Submit(job domain.Job) error {
	if !s.users.allow(job.UserID) {
		metrics.IncRejected()
		return domain.ErrRateLimited
	}

	select {
	case s.jobs <- job:
		metrics.IncRequest()
		metrics.SetQueueDepth(len(s.jobs))
		return nil
	default:
		metrics.IncRejected()
		return domain.ErrQueueFull
	}
}

// Run starts the worker pool and serves until ctx is cancelled.
func (s *Service) Run(ctx context.Context) error {
	err := os.MkdirAll(s.limits.WorkDir, 0o750)
	if err != nil {
		return fmt.Errorf("create work directory: %w", err)
	}

	var wg sync.WaitGroup

	for range s.limits.Workers {
		wg.Go(func() { s.worker(ctx) })
	}
	wg.Go(func() { s.evictLoop(ctx) })

	log.Info().Int("workers", s.limits.Workers).Int("queue", s.limits.QueueSize).Msg("download pipeline started")
	wg.Wait()
	log.Info().Msg("download pipeline stopped")

	return nil
}

func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.jobs:
			metrics.SetQueueDepth(len(s.jobs))
			s.process(ctx, job)
		}
	}
}

// process runs one job to completion. Every failure is reported to the user and
// logged here, at the layer that understands it; nothing propagates upward.
func (s *Service) process(ctx context.Context, job domain.Job) {
	ctx, cancel := context.WithTimeout(ctx, s.limits.DownloadTimeout)
	defer cancel()

	started := time.Now()

	err := s.run(ctx, job)
	if err != nil {
		// A cancelled job means the process is shutting down, not a failure
		// the user needs an explanation for.
		if ctx.Err() != nil && errors.Is(err, context.Canceled) {
			log.Warn().Str("query", job.Query).Msg("job abandoned during shutdown")
			return
		}

		metrics.IncDownloadFailed()
		log.Error().Err(err).Int64("user_id", job.UserID).Str("query", job.Query).Msg("job failed")

		// A job that failed by running out of time has an expired context, so
		// the apology needs one of its own or it never reaches the user.
		notifyCtx, notifyCancel := context.WithTimeout(context.WithoutCancel(ctx), notifyTimeout)
		s.out.Fail(notifyCtx, job, err)
		notifyCancel()

		return
	}

	metrics.IncDownload()
	log.Info().
		Int64("user_id", job.UserID).
		Str("query", job.Query).
		Dur("took", time.Since(started)).
		Msg("job delivered")
}

func (s *Service) run(ctx context.Context, job domain.Job) error {
	song, err := s.identify(ctx, job)
	if err != nil {
		return err
	}

	switch {
	case song.IsLive:
		return domain.ErrLiveStream
	case song.Duration > s.limits.MaxDuration:
		return domain.ErrTooLong
	}

	// A track anyone has asked for before is already on Telegram's servers.
	fileID, err := s.repo.FileID(ctx, song.ID)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	if fileID != "" {
		metrics.IncCacheHit()
		return s.out.DeliverCached(ctx, job, song, fileID)
	}

	return s.fetch(ctx, job, song)
}

// identify turns a request into a single track: a URL is resolved directly, a
// text query is searched first and the best match resolved.
func (s *Service) identify(ctx context.Context, job domain.Job) (domain.Song, error) {
	if job.IsURL {
		return s.yt.Resolve(ctx, job.Query)
	}

	err := s.out.Searching(ctx, job)
	if err != nil {
		return domain.Song{}, err
	}

	found, err := s.yt.Search(ctx, job.Query)
	if err != nil {
		return domain.Song{}, err
	}

	match, ok := bestMatch(found, job.Query, s.limits.MaxDuration)
	if !ok {
		return domain.Song{}, domain.ErrNotFound
	}

	return s.yt.Resolve(ctx, match.ID)
}

// fetch downloads a track into its own scratch directory and delivers it. The
// directory is removed whether or not the job succeeded.
func (s *Service) fetch(ctx context.Context, job domain.Job, song domain.Song) error {
	dir, err := os.MkdirTemp(s.limits.WorkDir, song.ID+"-")
	if err != nil {
		return fmt.Errorf("create job directory: %w", err)
	}
	defer func() {
		rmErr := os.RemoveAll(dir)
		if rmErr != nil {
			log.Error().Err(rmErr).Str("dir", dir).Msg("failed to clean up job directory")
		}
	}()

	err = s.out.Downloading(ctx, job, song, domain.Progress{})
	if err != nil {
		return err
	}

	audio, err := s.yt.Download(ctx, song, dir, s.progressReporter(ctx, job, song))
	if err != nil {
		return err
	}

	if audio.Size > s.limits.MaxFileSize {
		return domain.ErrTooLarge
	}

	err = s.out.Uploading(ctx, job, song)
	if err != nil {
		return err
	}

	fileID, err := s.out.Deliver(ctx, job, audio)
	if err != nil {
		return err
	}

	return s.repo.Save(ctx, song, fileID)
}

// progressReporter throttles yt-dlp's progress stream down to the handful of
// message edits a user actually benefits from.
func (s *Service) progressReporter(ctx context.Context, job domain.Job, song domain.Song) func(domain.Progress) {
	var (
		mu       sync.Mutex
		lastAt   time.Time
		lastPcnt float64
	)

	return func(p domain.Progress) {
		mu.Lock()
		if p.Percent-lastPcnt < progressStep || time.Since(lastAt) < progressMinGap {
			mu.Unlock()
			return
		}
		lastAt, lastPcnt = time.Now(), p.Percent
		mu.Unlock()

		err := s.out.Downloading(ctx, job, song, p)
		if err != nil {
			log.Debug().Err(err).Str("video_id", song.ID).Msg("failed to report progress")
		}
	}
}

func (s *Service) evictLoop(ctx context.Context) {
	ticker := time.NewTicker(evictInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.users.evictIdle()
		}
	}
}
