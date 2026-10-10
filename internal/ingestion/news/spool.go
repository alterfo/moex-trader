package news

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type ArchiveRecord struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
}

func ArticleID(link, title string) string {
	sum := sha256.Sum256([]byte(link + "|" + title))
	return hex.EncodeToString(sum[:])[:16]
}

func RecordKey(rec ArchiveRecord) string {
	return rec.Ticker + "|" + rec.ArticleID
}

func ReadRecords(path string) ([]ArchiveRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []ArchiveRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec ArchiveRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out, scanner.Err()
}

func AppendRecords(path string, records []ArchiveRecord) error {
	if len(records) == 0 {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, rec := range records {
		line, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	return w.Flush()
}

type Spool struct {
	path string
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewSpool(path string) (*Spool, error) {
	if path == "" {
		return nil, fmt.Errorf("news spool: path must not be empty")
	}
	existing, err := ReadRecords(path)
	if err != nil {
		return nil, fmt.Errorf("news spool: read %q: %w", path, err)
	}
	seen := make(map[string]struct{}, len(existing))
	for _, rec := range existing {
		seen[RecordKey(rec)] = struct{}{}
	}
	return &Spool{path: path, seen: seen}, nil
}

func (s *Spool) Add(matches []MatchedArticle) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fresh := make([]ArchiveRecord, 0)
	keys := make([]string, 0)
	for _, m := range matches {
		if m.Indirect {
			continue
		}
		rec := ArchiveRecord{
			Ticker:      m.Ticker,
			ArticleID:   ArticleID(m.Link, m.Title),
			Title:       m.Title,
			Source:      m.SourceName,
			TrustWeight: m.TrustWeight.InexactFloat64(),
			PublishedTS: m.PublishedAt.Unix(),
		}
		key := RecordKey(rec)
		if _, dup := s.seen[key]; dup {
			continue
		}
		s.seen[key] = struct{}{}
		fresh = append(fresh, rec)
		keys = append(keys, key)
	}
	if err := AppendRecords(s.path, fresh); err != nil {
		for _, key := range keys {
			delete(s.seen, key)
		}
		return 0, err
	}
	return len(fresh), nil
}

func MergeSpool(spoolPath, archivePath string) (merged, skipped int, err error) {
	pending := spoolPath + ".merging"
	if _, statErr := os.Stat(pending); statErr != nil {
		if renameErr := os.Rename(spoolPath, pending); renameErr != nil {
			if os.IsNotExist(renameErr) {
				return 0, 0, nil
			}
			return 0, 0, renameErr
		}
	}
	records, err := ReadRecords(pending)
	if err != nil {
		return 0, 0, err
	}
	archived, err := ReadRecords(archivePath)
	if err != nil {
		return 0, 0, err
	}
	seen := make(map[string]struct{}, len(archived))
	for _, rec := range archived {
		seen[RecordKey(rec)] = struct{}{}
	}
	fresh := make([]ArchiveRecord, 0, len(records))
	for _, rec := range records {
		key := RecordKey(rec)
		if _, dup := seen[key]; dup {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		fresh = append(fresh, rec)
	}
	if err := AppendRecords(archivePath, fresh); err != nil {
		return 0, 0, err
	}
	if err := os.Remove(pending); err != nil {
		return len(fresh), skipped, err
	}
	return len(fresh), skipped, nil
}
