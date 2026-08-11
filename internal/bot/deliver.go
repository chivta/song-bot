package bot

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/arvlas/song-bot/internal/domain"
)

// filenameTemplate is what the file is called once it lands on the user's
// device, for clients that show the name instead of the tags.
const filenameTemplate = "%s - %s%s"

// unsafeFilenameChars are stripped from a track's filename.
var unsafeFilenameChars = strings.NewReplacer("/", "-", "\\", "-", ":", "-", "\x00", "")

// Searching tells the user their query is being looked up.
func (b *Bot) Searching(ctx context.Context, job domain.Job) error {
	return b.editStatus(ctx, job, textSearching)
}

// Downloading reports transfer progress.
func (b *Bot) Downloading(ctx context.Context, job domain.Job, s domain.Song, p domain.Progress) error {
	return b.editStatus(ctx, job, downloading(s.Title, p))
}

// Uploading tells the user the transfer is done and the file is on its way.
func (b *Bot) Uploading(ctx context.Context, job domain.Job, _ domain.Song) error {
	return b.editStatus(ctx, job, textUploading)
}

// Deliver uploads a freshly downloaded track and returns the Telegram file ID
// it was stored under, which is what lets the next request skip the download.
func (b *Bot) Deliver(ctx context.Context, job domain.Job, a domain.Audio) (string, error) {
	audio := b.audio(a.Song)
	audio.File = tele.FromDisk(a.Path)
	audio.FileName = filename(a.Song, filepath.Ext(a.Path))

	if a.Cover != "" {
		audio.Thumbnail = &tele.Photo{File: tele.FromDisk(a.Cover)}
	}

	msg, err := b.send(ctx, job, audio)
	if err != nil {
		return "", fmt.Errorf("send audio: %w", err)
	}

	b.clearStatus(ctx, job)

	if msg.Audio == nil {
		// Telegram accepted the upload but classified it as something other
		// than audio, so there is no ID worth caching.
		log.Warn().Str("video_id", a.Song.ID).Msg("delivered file was not stored as audio")
		return "", nil
	}

	return msg.Audio.FileID, nil
}

// DeliverCached re-sends a track by the file ID Telegram already holds, which
// costs one API call and no download at all.
func (b *Bot) DeliverCached(ctx context.Context, job domain.Job, s domain.Song, fileID string) error {
	audio := b.audio(s)
	audio.File = tele.File{FileID: fileID}

	_, err := b.send(ctx, job, audio)
	if err != nil {
		return fmt.Errorf("send cached audio: %w", err)
	}

	b.clearStatus(ctx, job)

	return nil
}

// Fail replaces the status message with the reason the request went nowhere.
func (b *Bot) Fail(ctx context.Context, job domain.Job, cause error) {
	err := b.editStatus(ctx, job, describe(cause))
	if err != nil {
		log.Error().Err(err).Int64("chat_id", job.ChatID).Msg("failed to report job failure")
	}
}

// audio builds the message shared by both delivery paths. Title, performer and
// duration go in Telegram's own fields so clients render a real player.
func (b *Bot) audio(s domain.Song) *tele.Audio {
	return &tele.Audio{
		Duration:  int(s.Duration.Seconds()),
		Title:     s.Title,
		Performer: s.Artist,
		Caption:   caption(s),
	}
}

func (b *Bot) send(ctx context.Context, job domain.Job, audio *tele.Audio) (*tele.Message, error) {
	err := b.sends.wait(ctx)
	if err != nil {
		return nil, err
	}

	return b.bot.Send(&tele.Chat{ID: job.ChatID}, audio, tele.ModeHTML)
}

// editStatus rewrites the placeholder message the handler left behind. A failed
// edit is not worth failing a job over — the audio still arrives.
func (b *Bot) editStatus(ctx context.Context, job domain.Job, text string) error {
	err := b.sends.wait(ctx)
	if err != nil {
		return err
	}

	_, err = b.bot.Edit(statusMessage(job), text, tele.ModeHTML, tele.NoPreview)
	if err != nil {
		log.Debug().Err(err).Int64("chat_id", job.ChatID).Msg("failed to edit status message")
	}

	return nil
}

// clearStatus removes the placeholder once the track has been delivered.
func (b *Bot) clearStatus(ctx context.Context, job domain.Job) {
	err := b.sends.wait(ctx)
	if err != nil {
		return
	}

	err = b.bot.Delete(statusMessage(job))
	if err != nil {
		log.Debug().Err(err).Int64("chat_id", job.ChatID).Msg("failed to delete status message")
	}
}

func statusMessage(job domain.Job) tele.StoredMessage {
	return tele.StoredMessage{
		MessageID: strconv.Itoa(job.StatusMessageID),
		ChatID:    job.ChatID,
	}
}

func filename(s domain.Song, ext string) string {
	return unsafeFilenameChars.Replace(fmt.Sprintf(filenameTemplate, s.Artist, s.Title, ext))
}
