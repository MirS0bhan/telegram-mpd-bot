package processor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Processor does two things: download a file and run ffmpeg to produce a cleaned output
// This package intentionally exposes a small surface and keeps file handling self-contained.

type Processor struct {
	ffmpegPath string
	ffmpegArgs []string
	tmpDir     string
	storeDir   string
	log        *slog.Logger
}

func New(ffmpegPath string, ffmpegArgs []string, tmpDir string, storeDir string, logger *slog.Logger) *Processor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Processor{ffmpegPath: ffmpegPath, ffmpegArgs: ffmpegArgs, tmpDir: tmpDir, storeDir: storeDir, log: logger}
}

// sanitizeFilename keeps basic characters and ensures extension
func sanitizeFilename(hint string) string {
	if hint == "" {
		hint = "track"
	}
	ext := filepath.Ext(hint)
	base := strings.TrimSuffix(hint, ext)
	base = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ' ' {
			return r
		}
		return '-'
	}, base)
	if ext == "" {
		ext = ".mp3"
	}
	return base + ext
}

func (p *Processor) Process(ctx context.Context, fileURL string, filenameHint string) (string, error) {
	p.log.Info("processing incoming file", "filename_hint", filenameHint)

	// download to tmp file
	tmpFile, err := os.CreateTemp(p.tmpDir, "tmptel-*")
	if err != nil {
		p.log.Error("create temp file failed", "tmp_dir", p.tmpDir, "error", err)
		return "", fmt.Errorf("create temp: %w", err)
	}
	defer func() {
		if cerr := tmpFile.Close(); cerr != nil {
			p.log.Warn("temp file close failed", "error", cerr)
		}
	}()

	p.log.Debug("downloading file", "url", fileURL, "temp_file", tmpFile.Name())
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		p.log.Error("build download request failed", "url", fileURL, "error", err)
		return "", fmt.Errorf("create req: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.log.Error("download failed", "url", fileURL, "error", err)
		return "", fmt.Errorf("download: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			p.log.Warn("response body close failed", "error", cerr)
		}
	}()

	if resp.StatusCode != 200 {
		p.log.Error("unexpected download status", "url", fileURL, "status", resp.StatusCode)
		return "", fmt.Errorf("http status: %d", resp.StatusCode)
	}

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		p.log.Error("write temp file failed", "temp_file", tmpFile.Name(), "error", err)
		return "", fmt.Errorf("write tmp: %w", err)
	}
	p.log.Debug("download complete", "temp_file", tmpFile.Name(), "bytes", written)

	// prepare output path
	safeName := sanitizeFilename(filenameHint)
	outPath := filepath.Join(p.storeDir, fmt.Sprintf("%d-%s", time.Now().Unix(), safeName))

	// build ffmpeg args: -i <tmpFile> <ffmpegArgs> <outPath>
	args := []string{"-i", tmpFile.Name()}
	args = append(args, p.ffmpegArgs...)
	args = append(args, outPath)

	p.log.Debug("running ffmpeg", "path", p.ffmpegPath, "args", args)
	cmd := exec.CommandContext(ctx, p.ffmpegPath, args...)
	// keep logs in case of failure
	out, err := cmd.CombinedOutput()
	if err != nil {
		p.log.Error("ffmpeg failed", "error", err, "output", string(out))
		return "", fmt.Errorf("ffmpeg: %w", err)
	}
	p.log.Debug("ffmpeg output", "output", string(out))

	// success
	p.log.Info("processed file", "output_path", outPath)
	return outPath, nil
}
