package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Processor defines the minimal interface the telegram package needs to process an incoming file
type Processor interface {
	Process(ctx context.Context, fileURL string, filenameHint string) (storedPath string, err error)
}

// MPDController defines the minimal interface used by the telegram package
type MPDController interface {
	Enqueue(path string) error
}

// playlistJob carries a single message's download result through an ordered
// queue so that AddToPlaylist calls happen in the same order messages were
// received, even though downloads themselves run concurrently.
type playlistJob struct {
	chatID   int64
	filename string
	done     chan playlistResult
}

type playlistResult struct {
	stored string
	err    error
}

// Bot is the high-level orchestrator for Telegram updates
type Bot struct {
	api         *telegram.BotAPI
	allowedChat int64
	proc        Processor
	mpd         MPDController
	log         *slog.Logger
	jobs        chan *playlistJob
}

func New(token string, allowedChat int64, proc Processor, mpd MPDController, logger *slog.Logger) (*Bot, error) {
	if logger == nil {
		logger = slog.Default()
	}
	api, err := telegram.NewBotAPI(token)
	if err != nil {
		logger.Error("telegram new bot failed", "error", err)
		return nil, fmt.Errorf("telegram new bot: %w", err)
	}
	api.Debug = false
	logger.Info("telegram bot authorized", "username", api.Self.UserName)
	return &Bot{
		api:         api,
		allowedChat: allowedChat,
		proc:        proc,
		mpd:         mpd,
		log:         logger,
		jobs:        make(chan *playlistJob, 100),
	}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	u := telegram.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	go b.playlistWorker(ctx)

	b.log.Info("listening for updates")
	for {
		select {
		case upd := <-updates:
			if upd.Message == nil {
				continue
			}
			b.handleMessage(ctx, upd.Message)
		case <-ctx.Done():
			b.log.Info("shutdown signal received")
			return nil
		}
	}
}

// playlistWorker drains jobs strictly in the order they were enqueued
// (i.e. the order messages arrived), waiting for each job's download to
// finish before adding it to the playlist. This keeps playlist order
// stable even when multiple audio files are sent together (e.g. a
// Telegram media group / album) and downloaded concurrently.
func (b *Bot) playlistWorker(ctx context.Context) {
	for {
		select {
		case job, ok := <-b.jobs:
			if !ok {
				return
			}
			b.finishJob(job)
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bot) finishJob(job *playlistJob) {
	logger := b.log.With("chat_id", job.chatID, "filename_hint", job.filename)

	res := <-job.done
	if res.err != nil {
		logger.Error("process file failed", "error", res.err)
		b.reply("failed to process audio: "+res.err.Error(), job.chatID)
		return
	}
	logger.Debug("file processed", "stored_path", res.stored)

	if err := b.mpd.Enqueue(res.stored); err != nil {
		logger.Error("mpd add failed", "stored_path", res.stored, "error", err)
		b.reply("failed to add to playlist: "+err.Error(), job.chatID)
		return
	}

	logger.Info("added to playlist", "stored_path", res.stored)
	b.reply(fmt.Sprintf("Added to playlist: %s", filepath.Base(res.stored)), job.chatID)
}

func (b *Bot) handleMessage(ctx context.Context, m *telegram.Message) {
	logger := b.log
	var chatID int64
	if m.Chat != nil {
		chatID = m.Chat.ID
		logger = logger.With("chat_id", chatID, "message_id", m.MessageID)
	}

	// Only process if message is from allowed chat (if set)
	if b.allowedChat != 0 && m.Chat != nil && m.Chat.ID != b.allowedChat {
		logger.Warn("rejected message from disallowed chat", "allowed_chat_id", b.allowedChat)
		return
	}

	// Accept audio, voice, or document with audio mime
	var fileID string
	var filenameHint string
	switch {
	case m.Audio != nil:
		fileID = m.Audio.FileID
		filenameHint = m.Audio.FileName
	case m.Voice != nil:
		fileID = m.Voice.FileID
		filenameHint = "voice.ogg"
	case m.Document != nil:
		// basic mime check (HasPrefix avoids panicking on short mime types)
		if strings.HasPrefix(m.Document.MimeType, "audio") {
			fileID = m.Document.FileID
			filenameHint = m.Document.FileName
		}
	}

	if fileID == "" {
		logger.Debug("ignoring message without audio content")
		return
	}
	logger = logger.With("filename_hint", filenameHint)
	logger.Info("received audio message")

	// Enqueue the job now, synchronously, so playlist order matches the
	// order messages were received in (important when several audio files
	// arrive together, e.g. a Telegram media group). The actual download
	// happens concurrently in a goroutine and only fills in job.done;
	// playlistWorker consumes b.jobs in FIFO order.
	job := &playlistJob{
		chatID:   chatID,
		filename: filenameHint,
		done:     make(chan playlistResult, 1),
	}

	select {
	case b.jobs <- job:
	case <-ctx.Done():
		return
	}

	go b.downloadAndProcess(fileID, filenameHint, job)
}

func (b *Bot) downloadAndProcess(fileID, filenameHint string, job *playlistJob) {
	logger := b.log.With("chat_id", job.chatID, "filename_hint", filenameHint)

	// get file URL from telegram
	file, err := b.api.GetFile(telegram.FileConfig{FileID: fileID})
	if err != nil {
		logger.Error("get file failed", "error", err)
		job.done <- playlistResult{err: err}
		return
	}
	fileURL := file.Link(b.api.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	stored, err := b.proc.Process(ctx, fileURL, filenameHint)
	job.done <- playlistResult{stored: stored, err: err}
}

func (b *Bot) reply(text string, chatID int64) {
	if chatID == 0 {
		b.log.Warn("skipping reply: no chat id", "text", text)
		return
	}
	msg := telegram.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		b.log.Error("reply send failed", "chat_id", chatID, "error", err)
	}
}
