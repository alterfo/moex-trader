package news

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func matched(ticker, title string) MatchedArticle {
	return MatchedArticle{
		Ticker:      ticker,
		Title:       title,
		Link:        "https://example.test/" + title,
		PublishedAt: time.Unix(1790000000, 0),
		SourceName:  "РБК",
		TrustWeight: decimal.NewFromInt(1),
	}
}

func TestSpoolAddDedupesAcrossCyclesAndRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	spool, err := NewSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	batch := []MatchedArticle{matched("ROSN", "a"), matched("LKOH", "a"), matched("ROSN", "b")}
	if n, err := spool.Add(batch); err != nil || n != 3 {
		t.Fatalf("first Add() = %d, %v, want 3", n, err)
	}
	if n, err := spool.Add(batch); err != nil || n != 0 {
		t.Fatalf("repeat Add() = %d, %v, want 0", n, err)
	}
	reopened, err := NewSpool(path)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := reopened.Add(append(batch, matched("ROSN", "c"))); err != nil || n != 1 {
		t.Fatalf("Add() after restart = %d, %v, want 1", n, err)
	}
}

func TestSpoolSkipsIndirectMatches(t *testing.T) {
	spool, err := NewSpool(filepath.Join(t.TempDir(), "spool.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	indirect := matched("LKOH", "diesel")
	indirect.Indirect = true
	if n, err := spool.Add([]MatchedArticle{indirect}); err != nil || n != 0 {
		t.Fatalf("Add() = %d, %v, want indirect matches skipped", n, err)
	}
}

func TestMergeSpoolAppendsOnlyNewRecordsAndEmptiesSpool(t *testing.T) {
	dir := t.TempDir()
	spoolPath := filepath.Join(dir, "spool.jsonl")
	archivePath := filepath.Join(dir, "archive.jsonl")
	spool, err := NewSpool(spoolPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spool.Add([]MatchedArticle{matched("ROSN", "a"), matched("ROSN", "b")}); err != nil {
		t.Fatal(err)
	}
	existing := matched("ROSN", "a")
	if err := AppendRecords(archivePath, []ArchiveRecord{{Ticker: "ROSN", ArticleID: ArticleID(existing.Link, existing.Title)}}); err != nil {
		t.Fatal(err)
	}

	merged, skipped, err := MergeSpool(spoolPath, archivePath)
	if err != nil || merged != 1 || skipped != 1 {
		t.Fatalf("MergeSpool() = %d merged, %d skipped, %v, want 1/1", merged, skipped, err)
	}
	records, err := ReadRecords(archivePath)
	if err != nil || len(records) != 2 {
		t.Fatalf("archive has %d records, err=%v, want 2", len(records), err)
	}
	left, err := ReadRecords(spoolPath)
	if err != nil || len(left) != 0 {
		t.Fatalf("spool has %d records after merge, err=%v, want 0", len(left), err)
	}
	if merged, _, err := MergeSpool(spoolPath, archivePath); err != nil || merged != 0 {
		t.Fatalf("second MergeSpool() = %d, %v, want 0", merged, err)
	}
}
