package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/MirS0bhan/telegram-mpd-bot/internal/logging"
	mpd "github.com/MirS0bhan/telegram-mpd-bot/internal/mpd"
	"github.com/MirS0bhan/telegram-mpd-bot/internal/processor"
	telegrampkg "github.com/MirS0bhan/telegram-mpd-bot/internal/telegram"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram struct {
		Token         string `yaml:"token"`
		AllowedChatId int64  `yaml:"allowed_chat_id"`
	} `yaml:"telegram"`
	Ffmpeg struct {
		Path string   `yaml:"path"`
		Args []string `yaml:"args"`
	} `yaml:"ffmpeg"`
	MPD struct {
		Address        string `yaml:"address"`
		MusicDirectory string `yaml:"music_directory"`
		PlaylistName   string `yaml:"playlist_name"`
	} `yaml:"mpd"`
	Storage struct {
		TempDir          string `yaml:"temp_dir"`
		FilenameTemplate string `yaml:"filename_template"`
	} `yaml:"storage"`
	Logging logging.Config `yaml:"logging"`
}

func main() {
	cfgPath := flag.String("config", "config/config.yaml", "path to config.yaml")
	flag.Parse()

	b, err := os.ReadFile(*cfgPath)
	if err != nil {
		// No logger yet, config controls its own setup; fall back to a
		// plain stderr message for this one unavoidable bootstrap error.
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("read config", "error", err, "path", *cfgPath)
		os.Exit(1)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("parse config", "error", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.Logging, os.Stderr)

	// ensure temp dir exists
	if err := os.MkdirAll(cfg.Storage.TempDir, 0o755); err != nil {
		logger.Error("create temp dir", "error", err, "path", cfg.Storage.TempDir)
		os.Exit(1)
	}

	mpdClient := mpd.New(cfg.MPD.Address, cfg.MPD.PlaylistName, logging.Component(logger, "mpd"))
	proc := processor.New(cfg.Ffmpeg.Path, cfg.Ffmpeg.Args, cfg.Storage.TempDir, cfg.MPD.MusicDirectory, logging.Component(logger, "processor"))
	bot, err := telegrampkg.New(cfg.Telegram.Token, cfg.Telegram.AllowedChatId, proc, mpdClient, logging.Component(logger, "telegram"))
	if err != nil {
		logger.Error("telegram bot init", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting bot", "allowed_chat_id", cfg.Telegram.AllowedChatId, "mpd_address", cfg.MPD.Address)
	if err := bot.Start(ctx); err != nil {
		logger.Error("bot exited", "error", err)
		os.Exit(1)
	}
	logger.Info("bot stopped")
}
