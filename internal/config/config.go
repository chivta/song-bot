package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
)

// Config is the fully validated runtime configuration of the service.
type Config struct {
	// BotToken is the Telegram bot token from @BotFather.
	BotToken string `env:"BOT_TOKEN" validate:"required"`
	// AllowedUsers pins who may use the bot. Empty means anyone may.
	AllowedUsers []int64 `env:"ALLOWED_USERS" envSeparator:","`

	// DBPath holds the delivered-track cache. YTDLPDir holds the yt-dlp,
	// ffmpeg and ffprobe binaries; both belong on persistent storage.
	DBPath   string `env:"DB_PATH"   validate:"required"`
	YTDLPDir string `env:"YTDLP_DIR" validate:"required"`
	// WorkDir is scratch space for in-flight downloads; it is emptied per job.
	WorkDir string `env:"WORK_DIR" validate:"required"`
	// YTDLPPath overrides the managed yt-dlp binary with an explicit one. It is
	// the escape hatch for running a newer yt-dlp than this build ships with,
	// since YouTube extractor breakage moves faster than app releases.
	YTDLPPath string `env:"YTDLP_PATH"`

	// Workers bounds how many downloads run at once across all users, and
	// QueueSize how many may wait. Together they keep one busy user from
	// stalling everyone else.
	Workers   int `env:"WORKERS"    validate:"required,min=1,max=32"`
	QueueSize int `env:"QUEUE_SIZE" validate:"required,min=1"`

	// UserRatePerHour and UserBurst are the per-user token bucket. UserBurst
	// doubles as the cap on how many jobs one user may have in flight.
	UserRatePerHour int `env:"USER_RATE_PER_HOUR" validate:"required,min=1"`
	UserBurst       int `env:"USER_BURST"         validate:"required,min=1"`

	MaxDuration time.Duration `env:"MAX_DURATION" validate:"required,min=1m"`
	// MaxFileSizeMB is capped at 50: Telegram rejects larger bot uploads.
	MaxFileSizeMB   int64         `env:"MAX_FILE_SIZE_MB" validate:"required,min=1,max=50"`
	DownloadTimeout time.Duration `env:"DOWNLOAD_TIMEOUT" validate:"required,min=30s"`

	AudioFormat   string `env:"AUDIO_FORMAT"   validate:"required,oneof=m4a mp3 opus flac wav"`
	AudioQuality  string `env:"AUDIO_QUALITY"  validate:"required"`
	SearchResults int    `env:"SEARCH_RESULTS" validate:"required,min=1,max=25"`

	HTTPAddr string `env:"HTTP_ADDR" validate:"required"`
	LogLevel string `env:"LOG_LEVEL" validate:"required,oneof=debug info warn error"`
}

// Load reads .env when present, overlays the process environment and validates
// the result. Any problem is fatal for the caller.
func Load() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		DBPath:          "data/songbot.db",
		YTDLPDir:        "data/bin",
		WorkDir:         "/tmp/songbot",
		Workers:         3,
		QueueSize:       64,
		UserRatePerHour: 20,
		UserBurst:       3,
		MaxDuration:     20 * time.Minute,
		MaxFileSizeMB:   50,
		DownloadTimeout: 5 * time.Minute,
		AudioFormat:     "m4a",
		AudioQuality:    "0",
		SearchResults:   5,
		HTTPAddr:        ":8080",
		LogLevel:        "info",
	}

	err := env.Parse(&cfg)
	if err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}

	err = validator.New().Struct(cfg)
	if err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}
