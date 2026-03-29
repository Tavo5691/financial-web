package watcher

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"financial-web/internal/db/queries"
)

// Watcher polls a directory for new PDF files and registers them as pending statements.
type Watcher struct {
	InboxDir string
	DB       *sql.DB
	Logger   *slog.Logger
	ticker   *time.Ticker
}

// Start launches the background polling goroutine.
func (w *Watcher) Start(ctx context.Context) {
	w.ticker = time.NewTicker(5 * time.Second)
	go func() {
		w.Scan() // immediate scan on startup
		for {
			select {
			case <-w.ticker.C:
				w.Scan()
			case <-ctx.Done():
				w.ticker.Stop()
				return
			}
		}
	}()
}

// Scan walks the inbox directory and inserts new PDF files as pending statements.
// Already-seen files are skipped via INSERT OR IGNORE.
func (w *Watcher) Scan() {
	entries, err := os.ReadDir(w.InboxDir)
	if err != nil {
		if !os.IsNotExist(err) {
			w.Logger.Warn("inbox scan error", "dir", w.InboxDir, "error", err)
		}
		return
	}

	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
			continue
		}
		fullPath := filepath.Join(w.InboxDir, e.Name())
		hash, err := fileHash(fullPath)
		if err != nil {
			w.Logger.Warn("hash error", "file", fullPath, "error", err)
			continue
		}

		// INSERT OR IGNORE — if the row already exists nothing happens.
		id, err := queries.InsertPending(w.DB, fullPath, hash)
		if err != nil {
			w.Logger.Warn("insert pending", "file", fullPath, "error", err)
			continue
		}
		if id > 0 {
			w.Logger.Info("new PDF detected", "file", e.Name())
		}
	}
}

// fileHash returns the SHA-256 hex digest of a file.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
