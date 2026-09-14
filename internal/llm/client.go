package llm

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

const defaultHTTPTimeout = 30 * time.Second

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Format   any       `json:"format"`
}

type ChatResponse struct {
	Model   string  `json:"model"`
	Message Message `json:"message"`
	Done    bool    `json:"done"`
}

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func New(host, model string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &Client{
		baseURL:    normalizeBaseURL(host),
		model:      strings.TrimSpace(model),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Chat(ctx context.Context, messages []Message) (ChatResponse, error) {
	var empty ChatResponse
	payload, err := json.Marshal(ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Format:   TradeSignalSchema(),
	})
	if err != nil {
		return empty, fmt.Errorf("ollama: marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return empty, fmt.Errorf("ollama: build chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return empty, fmt.Errorf("ollama: chat %q: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return empty, fmt.Errorf("ollama: chat %q: unexpected status %d", c.baseURL, resp.StatusCode)
	}

	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return empty, fmt.Errorf("ollama: parse chat response %q: %w", c.baseURL, err)
	}
	return out, nil
}

func normalizeBaseURL(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimRight(host, "/")
	if host == "" {
		return host
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	return "http://" + host
}
