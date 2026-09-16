package failover

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func createTestDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE trades (id INTEGER PRIMARY KEY, ticker TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO trades (ticker) VALUES ('SBER')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
}

func TestSnapshotDBCopiesCommittedData(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "trader.db")
	dst := filepath.Join(dir, "snapshot.db")
	createTestDB(t, src)

	if err := SnapshotDB(context.Background(), src, dst); err != nil {
		t.Fatalf("SnapshotDB() error = %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+dst)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() {
		_ = db.Close()
	}()
	var ticker string
	if err := db.QueryRow(`SELECT ticker FROM trades`).Scan(&ticker); err != nil {
		t.Fatalf("query snapshot: %v", err)
	}
	if ticker != "SBER" {
		t.Fatalf("ticker = %q, want SBER", ticker)
	}
}

func TestSnapshotDBOverwritesStaleFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "trader.db")
	dst := filepath.Join(dir, "snapshot.db")
	createTestDB(t, src)
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	if err := SnapshotDB(context.Background(), src, dst); err != nil {
		t.Fatalf("SnapshotDB() error = %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if info.Size() == int64(len("stale")) {
		t.Fatalf("snapshot was not overwritten")
	}
}

func TestReplaceDBRemovesStaleSidecars(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "incoming")
	dst := filepath.Join(dir, "trader.db")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o644); err != nil {
		t.Fatalf("write destination: %v", err)
	}
	if err := os.WriteFile(dst+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatalf("write wal: %v", err)
	}
	if err := os.WriteFile(dst+"-shm", []byte("shm"), 0o644); err != nil {
		t.Fatalf("write shm: %v", err)
	}

	if err := ReplaceDB(src, dst); err != nil {
		t.Fatalf("ReplaceDB() error = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("destination = %q, want new", data)
	}
	for _, sidecar := range []string{dst + "-wal", dst + "-shm"} {
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("sidecar %q still exists", sidecar)
		}
	}
}

func TestDBFreshnessPrefersWal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trader.db")
	if err := os.WriteFile(path, []byte("db"), 0o644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	base := DBFreshness(path)
	if base.IsZero() {
		t.Fatalf("DBFreshness() = zero, want file mtime")
	}
	if err := os.WriteFile(path+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatalf("write wal: %v", err)
	}
	withWal := DBFreshness(path)
	if withWal.Before(base) {
		t.Fatalf("DBFreshness() with wal = %s, before %s", withWal, base)
	}
}
