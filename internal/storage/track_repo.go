package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/arvlas/song-bot/internal/domain"
)

// TrackRepo stores the Telegram file ID of every delivered track.
type TrackRepo struct {
	db *sql.DB
}

// NewTrackRepo builds a repository over an open database handle.
func NewTrackRepo(db *sql.DB) *TrackRepo {
	return &TrackRepo{db: db}
}

// FileID returns the Telegram file ID of an already delivered track, or
// domain.ErrNotFound when the track has never been sent.
func (r *TrackRepo) FileID(ctx context.Context, videoID string) (string, error) {
	var fileID string
	err := r.db.QueryRowContext(ctx, `SELECT file_id FROM tracks WHERE video_id = ?`, videoID).Scan(&fileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", domain.ErrNotFound
		}
		return "", fmt.Errorf("query track: %w", err)
	}

	return fileID, nil
}

// Save records a delivered track. A track re-uploaded under a new file ID
// replaces the old row.
func (r *TrackRepo) Save(ctx context.Context, s domain.Song, fileID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO tracks (video_id, title, artist, duration, file_id, created_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(video_id) DO UPDATE SET
		     title = excluded.title, artist = excluded.artist,
		     duration = excluded.duration, file_id = excluded.file_id`,
		s.ID, s.Title, s.Artist, int64(s.Duration.Seconds()), fileID, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert track: %w", err)
	}

	return nil
}
