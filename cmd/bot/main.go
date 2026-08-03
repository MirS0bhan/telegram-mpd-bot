package main

import (
	"context"
	"flag"
	"fmt"
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
	if err := run(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "config/config.yaml", "path to config.yaml")
	flag.Parse()

	b, err := os.ReadFile(*cfgPath)
	if err != nil {
		// No logger yet, config controls its own setup; fall back to a
		// plain stderr message for this one unavoidable bootstrap error.
		return fmt.Errorf("read config %q: %w", *cfgPath, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	logger := logging.New(cfg.Logging, os.Stderr)

	// ensure temp dir exists
	if err := os.MkdirAll(cfg.Storage.TempDir, 0o755); err != nil {
		return fmt.Errorf("create temp dir %q: %w", cfg.Storage.TempDir, err)
	}

	mpdClient := mpd.New(cfg.MPD.Address, cfg.MPD.MusicDirectory, logging.Component(logger, "mpd"))
	proc := processor.New(cfg.Ffmpeg.Path, cfg.Ffmpeg.Args, cfg.Storage.TempDir, cfg.MPD.MusicDirectory, logging.Component(logger, "processor"))
	bot, err := telegrampkg.New(cfg.Telegram.Token, cfg.Telegram.AllowedChatId, proc, mpdClient, logging.Component(logger, "telegram"))
	if err != nil {
		return fmt.Errorf("telegram bot init: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting bot", "allowed_chat_id", cfg.Telegram.AllowedChatId, "mpd_address", cfg.MPD.Address)
	if err := bot.Start(ctx); err != nil {
		return fmt.Errorf("bot exited: %w", err)
	}
	logger.Info("bot stopped")
	return nil
}
