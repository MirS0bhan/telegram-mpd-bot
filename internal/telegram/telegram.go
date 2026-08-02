package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	telegram "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Processor defines the minimal interface the telegram package needs to process an incoming file
type Processor interface {
	Process(ctx context.Context, fileURL string, filenameHint string) (storedPath string, err error)
}

// MPDController defines the minimal interface used by the telegram package
type MPDController interface {
	AddToPlaylist(path string) error
}

// Bot is the high-level orchestrator for Telegram updates
type Bot struct {
	api         *telegram.BotAPI
	allowedChat int64
	proc        Processor
	mpd         MPDController
	log         *slog.Logger
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
	return &Bot{api: api, allowedChat: allowedChat, proc: proc, mpd: mpd, log: logger}, nil
}

func (b *Bot) Start(ctx context.Context) error {
	u := telegram.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	b.log.Info("listening for updates")
	for {
		select {
		case upd := <-updates:
			if upd.Message == nil {
				continue
			}
			go b.handleMessage(upd.Message)
		case <-ctx.Done():
			b.log.Info("shutdown signal received")
			return nil
		}
	}
}

func (b *Bot) handleMessage(m *telegram.Message) {
	logger := b.log
	if m.Chat != nil {
		logger = logger.With("chat_id", m.Chat.ID, "message_id", m.MessageID)
	}

	// Only process if message is from allowed chat (if set)
	if b.allowedChat != 0 && m.Chat != nil && m.Chat.ID != b.allowedChat {
		logger.Warn("rejected message from disallowed chat", "allowed_chat_id", b.allowedChat)
		return
	}

	// Accept audio, voice, or document with audio mime
	var fileID string
	var filenameHint string
	if m.Audio != nil {
		fileID = m.Audio.FileID
		filenameHint = m.Audio.FileName
	} else if m.Voice != nil {
		fileID = m.Voice.FileID
		filenameHint = "voice.ogg"
	} else if m.Document != nil {
		// basic mime check
		if m.Document.MimeType != "" && m.Document.MimeType[:5] == "audio" {
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

	// get file URL from telegram
	file, err := b.api.GetFile(telegram.FileConfig{FileID: fileID})
	if err != nil {
		logger.Error("get file failed", "error", err)
		return
	}
	fileURL := file.Link(b.api.Token)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	stored, err := b.proc.Process(ctx, fileURL, filenameHint)
	if err != nil {
		logger.Error("process file failed", "error", err)
		b.reply("failed to process audio: "+err.Error(), m.Chat.ID)
		return
	}
	logger.Debug("file processed", "stored_path", stored)

	// Add to MPD playlist (minimal control)
	if err := b.mpd.AddToPlaylist(stored); err != nil {
		logger.Error("mpd add failed", "stored_path", stored, "error", err)
		b.reply("failed to add to playlist: "+err.Error(), m.Chat.ID)
		return
	}

	logger.Info("added to playlist", "stored_path", stored)
	b.reply(fmt.Sprintf("Added to playlist: %s", filepath.Base(stored)), m.Chat.ID)
}

func (b *Bot) reply(text string, chatID int64) {
	msg := telegram.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		b.log.Error("reply send failed", "chat_id", chatID, "error", err)
	}
}
