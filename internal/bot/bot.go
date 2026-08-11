package bot

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	tele "gopkg.in/telebot.v4"

	"github.com/arvlas/song-bot/internal/domain"
)

const (
	// pollerTimeout is how long a long-poll request waits for an update.
	pollerTimeout = 10 * time.Second
	// sendRate and sendBurst keep every outbound call, across all chats and
	// workers, inside Telegram's global 30-per-second ceiling.
	sendRate  = 25
	sendBurst = 5
)

// submitter is the download pipeline seen from the delivery layer.
type submitter interface {
	Submit(job domain.Job) error
}

// Bot is the Telegram delivery layer. It accepts requests, hands them to the
// pipeline and renders whatever comes back; it holds no business logic.
type Bot struct {
	bot     *tele.Bot
	jobs    submitter
	allowed map[int64]bool
	sends   *sendLimiter
}

// New builds the bot. An empty allowedUsers means anyone may use it.
func New(token string, allowedUsers []int64) (*Bot, error) {
	inner, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: pollerTimeout},
		OnError: func(err error, c tele.Context) {
			log.Error().Err(err).Msg("telegram handler failed")
		},
	})
	if err != nil {
		return nil, err
	}

	return &Bot{
		bot:     inner,
		allowed: allowList(allowedUsers),
		sends:   newSendLimiter(sendRate, sendBurst),
	}, nil
}

// Run registers the handlers and serves updates until ctx is cancelled.
func (b *Bot) Run(ctx context.Context, jobs submitter) error {
	b.jobs = jobs

	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/help", b.handleStart)
	b.bot.Handle(tele.OnText, b.handleRequest)

	go func() {
		<-ctx.Done()
		b.bot.Stop()
	}()

	log.Info().Str("username", b.bot.Me.Username).Msg("telegram bot listening")
	b.bot.Start()
	log.Info().Msg("telegram bot stopped")

	return nil
}

func (b *Bot) handleStart(c tele.Context) error {
	if !b.authorized(c) {
		return c.Send(describe(domain.ErrUnauthorized))
	}

	return c.Send(textStart, tele.ModeHTML, tele.NoPreview)
}

// handleRequest turns a message into a queued job. It answers immediately with
// a placeholder the workers then edit, so the handler never waits on a download.
func (b *Bot) handleRequest(c tele.Context) error {
	if !b.authorized(c) {
		return c.Send(describe(domain.ErrUnauthorized))
	}

	query, isURL, err := classify(c.Text())
	if err != nil {
		return c.Send(describe(err))
	}
	if query == "" {
		return c.Send(textEmptyQuery)
	}

	status, err := c.Bot().Send(c.Chat(), textQueued)
	if err != nil {
		return err
	}

	job := domain.Job{
		UserID:          c.Sender().ID,
		ChatID:          c.Chat().ID,
		StatusMessageID: status.ID,
		Query:           query,
		IsURL:           isURL,
		RequestedAt:     time.Now(),
	}

	err = b.jobs.Submit(job)
	if err != nil {
		log.Info().Err(err).Int64("user_id", job.UserID).Msg("request rejected")
		_, editErr := c.Bot().Edit(status, describe(err))

		return editErr
	}

	log.Info().Int64("user_id", job.UserID).Str("query", query).Bool("url", isURL).Msg("request queued")

	return nil
}

// authorized reports whether the sender of an update may use the bot.
func (b *Bot) authorized(c tele.Context) bool {
	sender := c.Sender()
	if sender == nil {
		// An update with no identifiable sender is only let through when the
		// bot is public anyway.
		return len(b.allowed) == 0
	}

	return b.allows(sender.ID)
}

// allows reports whether a user ID is on the allow list. An empty list means
// no list was configured, which makes the bot public.
func (b *Bot) allows(id int64) bool {
	if len(b.allowed) == 0 {
		return true
	}

	return b.allowed[id]
}

// allowList indexes the configured user IDs for lookup.
func allowList(ids []int64) map[int64]bool {
	allowed := make(map[int64]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}

	return allowed
}
