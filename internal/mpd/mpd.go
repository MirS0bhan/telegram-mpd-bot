package mpdclient

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/fhs/gompd/v2/mpd"
)

// Minimal MPD controller: add to the live play queue, or to a stored playlist.

type Client interface {
	AddToPlaylist(path string) error
}

type client struct {
	addr     string
	playlist string
	musicDir string
	log      *slog.Logger
}

// New creates an MPD client.
//
// musicDir must match MPD's own music_directory setting (from mpd.conf).
// Paths passed to AddToPlaylist are made relative to it before being sent
// to MPD, since MPD's protocol expects a URI relative to music_directory
// (or a URL) - never an absolute filesystem path.
//
// playlist controls what AddToPlaylist does:
//   - empty: tracks are appended to MPD's live play queue (so they play
//     soon), via the "add" command.
//   - non-empty: tracks are appended to that *stored* playlist file
//     instead, via "playlistadd". Note this does NOT touch what's
//     currently playing - something else has to `load` it later.
func New(addr, playlist, musicDir string, logger *slog.Logger) Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &client{addr: addr, playlist: playlist, musicDir: musicDir, log: logger}
}

// toRelative converts an absolute path under musicDir into the
// music_directory-relative form MPD expects. If p isn't under musicDir (or
// musicDir isn't configured), p is returned unchanged - this covers callers
// that already hand back MPD-relative paths.
func (c *client) toRelative(p string) string {
	if c.musicDir == "" || !filepath.IsAbs(p) {
		return p
	}
	rel, err := filepath.Rel(c.musicDir, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		c.log.Warn("path is not under configured music_directory; sending as-is",
			"path", p, "music_dir", c.musicDir)
		return p
	}
	return rel
}

// AddToPlaylist connects and issues a single add command. We keep the control surface minimal.
func (c *client) AddToPlaylist(path string) error {
	p := c.toRelative(path)

	c.log.Debug("connecting to mpd", "address", c.addr)
	conn, err := mpd.Dial("tcp", c.addr)
	if err != nil {
		c.log.Error("connect mpd failed", "address", c.addr, "error", err)
		return fmt.Errorf("connect mpd: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			c.log.Warn("mpd connection close failed", "error", cerr)
		}
	}()

	if c.playlist == "" {
		c.log.Debug("adding to live queue", "path", p)
		if err := conn.Add(p); err != nil {
			c.log.Error("queue add failed", "path", p, "error", err)
			return fmt.Errorf("queue add: %w", err)
		}
		c.log.Info("added track to live queue", "path", p)
		return nil
	}

	c.log.Debug("adding to stored playlist", "playlist", c.playlist, "path", p)
	if err := conn.PlaylistAdd(c.playlist, p); err != nil {
		c.log.Error("playlist add failed", "playlist", c.playlist, "path", p, "error", err)
		return fmt.Errorf("playlist add: %w", err)
	}
	c.log.Info("added track to stored playlist", "playlist", c.playlist, "path", p)
	return nil
}
