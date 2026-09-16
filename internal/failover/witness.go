package failover

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type LeaseStatus struct {
	Reachable bool
	Holder    string
	ExpiresAt time.Time
	TTL       time.Duration
}

func (l LeaseStatus) HeldBy(node string, now time.Time) bool {
	return l.Reachable && l.Holder == node && now.Before(l.ExpiresAt)
}

func (l LeaseStatus) HeldByOther(node string, now time.Time) bool {
	return l.Reachable && l.Holder != "" && l.Holder != node && now.Before(l.ExpiresAt)
}

type WitnessClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewWitnessClient(baseURL, token string, client *http.Client) *WitnessClient {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &WitnessClient{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Token:   strings.TrimSpace(token),
		HTTP:    client,
	}
}

type witnessPayload struct {
	Node       string  `json:"node,omitempty"`
	Holder     string  `json:"holder"`
	ExpiresAt  string  `json:"expires_at,omitempty"`
	Granted    bool    `json:"granted"`
	TTLSeconds float64 `json:"ttl_seconds,omitempty"`
}

func (c *WitnessClient) Status(ctx context.Context) (LeaseStatus, error) {
	status, err := c.do(ctx, http.MethodGet, "/status", "")
	if err != nil {
		return LeaseStatus{}, err
	}
	return status, nil
}

func (c *WitnessClient) Renew(ctx context.Context, node string) (LeaseStatus, bool, error) {
	body, err := json.Marshal(witnessPayload{Node: node})
	if err != nil {
		return LeaseStatus{}, false, fmt.Errorf("witness renew: marshal request: %w", err)
	}
	status, err := c.do(ctx, http.MethodPost, "/renew", string(body))
	if err != nil {
		return LeaseStatus{}, false, err
	}
	return status, status.Holder == node, nil
}

func (c *WitnessClient) Release(ctx context.Context, node string) error {
	body, err := json.Marshal(witnessPayload{Node: node})
	if err != nil {
		return fmt.Errorf("witness release: marshal request: %w", err)
	}
	_, err = c.do(ctx, http.MethodPost, "/release", string(body))
	return err
}

func (c *WitnessClient) do(ctx context.Context, method, path, body string) (LeaseStatus, error) {
	if c.BaseURL == "" {
		return LeaseStatus{}, fmt.Errorf("witness: base url is empty")
	}
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return LeaseStatus{}, fmt.Errorf("witness %s: build request: %w", path, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return LeaseStatus{}, fmt.Errorf("witness %s: %w", path, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return LeaseStatus{}, fmt.Errorf("witness %s: unexpected status %s", path, resp.Status)
	}
	var payload witnessPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return LeaseStatus{}, fmt.Errorf("witness %s: decode response: %w", path, err)
	}
	status := LeaseStatus{
		Reachable: true,
		Holder:    payload.Holder,
		TTL:       time.Duration(payload.TTLSeconds * float64(time.Second)),
	}
	if payload.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
		if err != nil {
			return LeaseStatus{}, fmt.Errorf("witness %s: parse expires_at: %w", path, err)
		}
		status.ExpiresAt = expiresAt
	}
	return status, nil
}
