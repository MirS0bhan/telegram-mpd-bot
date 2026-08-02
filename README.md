# Telegram -> FFmpeg -> MPD bridge (Go)

This project implements a Telegram bot that listens for new audio sent in a configured group, downloads it, runs a configurable ffmpeg processing pipeline to "make it better", stores the resulting track in MPD's music directory, and adds it to MPD's playlist. The code is structured with clear separation of concerns and abstraction layers.

What I added:
- cmd/bot/main.go — application entrypoint and wiring
- internal/telegram — Telegram update handling and download orchestration
- internal/processor — file download + ffmpeg processing logic
- internal/mpd — thin MPD client wrapper to add files to playlist
- internal/logging — leveled, structured logging (log/slog) shared across all components
- config/config.yaml — example configuration (includes MPD settings)
- config/mpd.conf — example MPD configuration with HTTP streaming

Requirements
- ffmpeg installed and on PATH (or set in config)
- mpd installed and configured to use the provided mpd.conf or merge settings
- a Telegram Bot token with permission to read messages in the target group

Run (example):
1. Edit config/config.yaml
2. start mpd with config/mpd.conf (or install/merge settings)
3. go run ./cmd/bot

Notes on design
- Maximum layering: interfaces for Processor and MPD client
- Minimal MPD control: only AddToPlaylist is used by the bot
- Files are stored in a configured music_directory that MPD serves

Logging
- Structured, multi-level logging is built on Go's `log/slog` (see `internal/logging`).
- Every component (telegram, processor, mpd) gets its own logger tagged with `component=<name>`, so log lines can be filtered by subsystem.
- Levels used: `debug` (verbose per-request detail like URLs, byte counts, ffmpeg args), `info` (lifecycle events: bot start/stop, message received, file processed, playlist updates), `warn` (rejected/ignored input, e.g. messages from disallowed chats), `error` (failures such as download, ffmpeg, MPD, or Telegram API errors).
- Configure via `config.yaml`:
  ```yaml
  logging:
    level: "info"   # debug | info | warn | error
    format: "text"  # text | json
  ```
- Set `level: debug` for troubleshooting and `format: json` when shipping logs to an aggregator.


```
docker run -d --name telegram-mpd-bot -v path/to/radio/music:/music:ro -v path/to/radio/config:/app/config:ro -v mpd-data:/var/lib/mpd -p 8000:8000 -p 6600:6600 ghcr.io/mirs0bhan/telegram-mpd-bot:latest 
```