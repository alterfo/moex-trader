package failover

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

func SnapshotDB(ctx context.Context, src, dst string) error {
	if strings.TrimSpace(src) == "" {
		return fmt.Errorf("snapshot db: source path is empty")
	}
	if strings.TrimSpace(dst) == "" {
		return fmt.Errorf("snapshot db: destination path is empty")
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("snapshot db: remove stale %q: %w", dst, err)
	}
	db, err := sql.Open("sqlite", "file:"+src+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("snapshot db: open %q: %w", src, err)
	}
	defer func() {
		_ = db.Close()
	}()
	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, dst); err != nil {
		return fmt.Errorf("snapshot db: vacuum %q into %q: %w", src, dst, err)
	}
	return nil
}

func DBFreshness(path string) time.Time {
	var freshest time.Time
	for _, candidate := range []string{path, path + "-wal"} {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.ModTime().After(freshest) {
			freshest = info.ModTime()
		}
	}
	return freshest
}

func ReplaceDB(src, dst string) error {
	if strings.TrimSpace(src) == "" {
		return fmt.Errorf("replace db: source path is empty")
	}
	if strings.TrimSpace(dst) == "" {
		return fmt.Errorf("replace db: destination path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("replace db: create directory for %q: %w", dst, err)
	}
	for _, sidecar := range []string{dst + "-wal", dst + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("replace db: remove %q: %w", sidecar, err)
		}
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("replace db: rename %q to %q: %w", src, dst, err)
	}
	return nil
}
