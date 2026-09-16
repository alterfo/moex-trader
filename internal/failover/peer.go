package failover

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type PeerClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewPeerClient(baseURL, token string, client *http.Client) *PeerClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &PeerClient{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Token:   strings.TrimSpace(token),
		HTTP:    client,
	}
}

func (c *PeerClient) Heartbeat(ctx context.Context) (Heartbeat, error) {
	if c.BaseURL == "" {
		return Heartbeat{}, fmt.Errorf("peer: base url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return Heartbeat{}, fmt.Errorf("peer heartbeat: build request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Heartbeat{}, fmt.Errorf("peer heartbeat: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return Heartbeat{}, fmt.Errorf("peer heartbeat: unexpected status %s", resp.Status)
	}
	var heartbeat Heartbeat
	if err := json.NewDecoder(resp.Body).Decode(&heartbeat); err != nil {
		return Heartbeat{}, fmt.Errorf("peer heartbeat: decode response: %w", err)
	}
	return heartbeat, nil
}

func (c *PeerClient) RequestYield(ctx context.Context) error {
	return c.control(ctx, "/control/yield")
}

func (c *PeerClient) control(ctx context.Context, path string) error {
	if c.BaseURL == "" {
		return fmt.Errorf("peer: base url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("peer %s: build request: %w", path, err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("peer %s: %w", path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("peer %s: unexpected status %s", path, resp.Status)
	}
	return nil
}

func (c *PeerClient) FetchDB(ctx context.Context, dst string, force bool) (bool, error) {
	if c.BaseURL == "" {
		return false, fmt.Errorf("peer: base url is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/db", nil)
	if err != nil {
		return false, fmt.Errorf("peer db: build request: %w", err)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if local := DBFreshness(dst); !local.IsZero() && !force {
		req.Header.Set("If-Modified-Since", local.UTC().Format(http.TimeFormat))
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return false, fmt.Errorf("peer db: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("peer db: unexpected status %s", resp.Status)
	}
	tmp := dst + ".incoming"
	file, err := os.Create(tmp)
	if err != nil {
		return false, fmt.Errorf("peer db: create %q: %w", tmp, err)
	}
	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return false, fmt.Errorf("peer db: download: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return false, fmt.Errorf("peer db: sync %q: %w", tmp, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return false, fmt.Errorf("peer db: close %q: %w", tmp, err)
	}
	if err := validateSQLiteFile(tmp); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	modified := time.Now()
	if header := resp.Header.Get("Last-Modified"); header != "" {
		if parsed, err := http.ParseTime(header); err == nil {
			modified = parsed
		}
	}
	if err := ReplaceDB(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	_ = os.Chtimes(dst, modified, modified)
	return true, nil
}

func validateSQLiteFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("peer db: validate %q: %w", path, err)
	}
	defer func() {
		_ = file.Close()
	}()
	header := make([]byte, 16)
	if _, err := io.ReadFull(file, header); err != nil {
		return fmt.Errorf("peer db: read header of %q: %w", path, err)
	}
	if string(header) != "SQLite format 3\x00" {
		return fmt.Errorf("peer db: %q is not a sqlite database", path)
	}
	return nil
}
