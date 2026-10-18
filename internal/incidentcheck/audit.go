package incidentcheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditRecord struct {
	At        time.Time `json:"at"`
	Ticker    string    `json:"ticker"`
	Model     string    `json:"model,omitempty"`
	Digest    string    `json:"model_digest,omitempty"`
	PromptSHA string    `json:"prompt_sha,omitempty"`
	Titles    []string  `json:"titles"`
	Raw       string    `json:"raw,omitempty"`
	Verdict   string    `json:"verdict,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Outcome   string    `json:"outcome"`
	Error     string    `json:"error,omitempty"`
}

type AuditLog struct {
	path string
	mu   sync.Mutex
}

func NewAuditLog(path string) *AuditLog {
	if path == "" {
		return nil
	}
	return &AuditLog{path: path}
}

func (a *AuditLog) Append(rec AuditRecord) error {
	if a == nil {
		return nil
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("incident audit: marshal: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return fmt.Errorf("incident audit: mkdir: %w", err)
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("incident audit: open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		return fmt.Errorf("incident audit: write: %w", err)
	}
	return f.Sync()
}
