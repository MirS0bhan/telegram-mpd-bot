package mpdclient

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fhs/gompd/v2/mpd"
)

// Client appends tracks to MPD's live queue and starts playback only if MPD
// is currently stopped.
type Client interface {
	Enqueue(path string) error
}

type client struct {
	addr     string
	musicDir string
	log      *slog.Logger
}

// New creates an MPD queue client.
//
// musicDir must match MPD's music_directory from mpd.conf.
func New(addr, musicDir string, logger *slog.Logger) Client {
	if logger == nil {
		logger = slog.Default()
	}

	return &client{
		addr:     addr,
		musicDir: filepath.Clean(musicDir),
		log:      logger,
	}
}

// toRelative converts an absolute filesystem path into the MPD URI relative
// to music_directory.
func (c *client) toRelative(p string) (string, error) {
	if c.musicDir == "" {
		return "", fmt.Errorf("musicDir is not configured")
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("abs path: %w", err)
	}

	rel, err := filepath.Rel(c.musicDir, abs)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}

	if rel == "." || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q is outside musicDir %q", abs, c.musicDir)
	}

	// MPD expects forward slashes even on Windows.
	return filepath.ToSlash(rel), nil
}

// waitForUpdate blocks until MPD finishes its database update.
func waitForUpdate(conn *mpd.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := conn.Status()
		if err != nil {
			return fmt.Errorf("status during update wait: %w", err)
		}

		// When updating_db disappears, the update has finished.
		if status["updating_db"] == "" {
			return nil
		}

		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("mpd update timed out after %s", timeout)
}

// Enqueue updates MPD's database for the file's directory, appends the track
// to the live queue, and starts playback only if MPD is currently stopped.
func (c *client) Enqueue(path string) error {
	uri, err := c.toRelative(path)
	if err != nil {
		return err
	}

	c.log.Debug("enqueue request", "path", path, "uri", uri)

	conn, err := mpd.Dial("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("connect mpd: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			c.log.Warn("mpd connection close failed", "error", cerr)
		}
	}()

	// Update only the containing directory, not the whole library.
	dir := filepath.ToSlash(filepath.Dir(uri))
	if dir == "." {
		dir = ""
	}

	c.log.Debug("updating mpd directory", "dir", dir)

	jobID, err := conn.Update(dir)
	if err != nil {
		return fmt.Errorf("mpd update %q: %w", dir, err)
	}

	c.log.Debug("waiting for mpd update", "job_id", jobID)

	if err := waitForUpdate(conn, 5*time.Second); err != nil {
		return err
	}

	c.log.Debug("mpd update finished", "job_id", jobID)

	// Append to queue and get the queue song ID.
	id, err := conn.AddID(uri, -1)
	if err != nil {
		return fmt.Errorf("mpd addid %q: %w", uri, err)
	}

	c.log.Debug("track added to queue", "uri", uri, "id", id)

	status, err := conn.Status()
	if err != nil {
		return fmt.Errorf("mpd status: %w", err)
	}

	state := status["state"]

	// Start playback only if MPD is currently stopped.
	if state == "stop" {
		if err := conn.PlayID(id); err != nil {
			return fmt.Errorf("mpd playid %d: %w", id, err)
		}

		c.log.Info("track enqueued and playback started",
			"uri", uri,
			"id", id,
		)

		return nil
	}

	c.log.Info("track enqueued",
		"uri", uri,
		"id", id,
		"state", state,
	)

	return nil
}
