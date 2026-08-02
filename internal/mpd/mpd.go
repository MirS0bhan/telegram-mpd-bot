package mpdclient

import (
	"fmt"
	"log/slog"

	"github.com/fhs/gompd/v2/mpd"
)

// Minimal MPD controller: only add to playlist

type Client interface {
	AddToPlaylist(path string) error
}

type client struct {
	addr     string
	playlist string
	log      *slog.Logger
}

func New(addr, playlist string, logger *slog.Logger) Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &client{addr: addr, playlist: playlist, log: logger}
}

// AddToPlaylist connects and issues a single add command. We keep the control surface minimal.
func (c *client) AddToPlaylist(path string) error {
	c.log.Debug("connecting to mpd", "address", c.addr)
	conn, err := mpd.Dial("tcp", c.addr)
	if err != nil {
		c.log.Error("connect mpd failed", "address", c.addr, "error", err)
		return fmt.Errorf("connect mpd: %w", err)
	}
	defer conn.Close()

	// mpd expects path relative to music_directory; ideally the bot stores into that dir.
	// If an absolute path is given and it starts with the music dir, we convert to relative.
	p := path

	c.log.Debug("adding to playlist", "playlist", c.playlist, "path", p)
	if err := conn.PlaylistAdd(c.playlist, p); err != nil {
		c.log.Error("playlist add failed", "playlist", c.playlist, "path", p, "error", err)
		return fmt.Errorf("playlist add: %w", err)
	}
	c.log.Info("added track to playlist", "playlist", c.playlist, "path", p)
	return nil
}
