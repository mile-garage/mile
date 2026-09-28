// Package backup makes a daily copy of the database. Attachments are plain
// files in the data directory: back up the whole volume (e.g. with a ZFS
// snapshot) to keep them too.
package backup

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const prefix = "mile-"

// Run makes one backup a day (VACUUM INTO, safe while the database is in
// use) and keeps the last keep files.
func Run(ctx context.Context, db *sql.DB, dir string, keep int, daily func()) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("backup: directory", "err", err)
		return
	}
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()
	for {
		if err := Today(db, dir, keep); err != nil {
			slog.Error("backup", "err", err)
		}
		if daily != nil {
			daily()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func Today(db *sql.DB, dir string, keep int) error {
	path := filepath.Join(dir, prefix+time.Now().Format("2006-01-02")+".db")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if _, err := db.Exec(`VACUUM INTO ?`, path); err != nil {
		return err
	}
	slog.Info("backup created", "file", path)
	return prune(dir, keep)
}

func prune(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".db") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)
	for len(files) > keep {
		if err := os.Remove(filepath.Join(dir, files[0])); err != nil {
			return err
		}
		files = files[1:]
	}
	return nil
}
