# Telegram → FFmpeg → MPD Bot

A Go-based Telegram bot that listens for audio messages, processes them through FFmpeg, stores the processed tracks in an MPD music directory, and automatically adds them to the MPD playback queue.

Example configuration files are available in the repository under `config/` (`config.yaml` and `mpd.conf`).

```bash
docker run -d \
  --name telegram-mpd-bot \
  -v /path/to/music:/music:ro \
  -v /path/to/config:/app/config:ro \
  -v mpd-data:/var/lib/mpd \
  -p 8000:8000 \ # for online streaming 
  -p 6600:6600 \ # to manage MPD it self
  ghcr.io/mirs0bhan/telegram-mpd-bot:latest