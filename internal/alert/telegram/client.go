package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.telegram.org"

type sendMessageResponse struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
}

type Client struct {
	baseURL    string
	token      string
	chatID     string
	httpClient *http.Client
}

func New(token, chatID string, httpClient *http.Client) *Client {
	return newClient(defaultBaseURL, token, chatID, httpClient)
}

func newClient(baseURL, token, chatID string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:      strings.TrimSpace(token),
		chatID:     strings.TrimSpace(chatID),
		httpClient: httpClient,
	}
}

func (c *Client) Enabled() bool {
	return c != nil && c.token != "" && c.chatID != ""
}

func (c *Client) Send(ctx context.Context, text string) error {
	if !c.Enabled() {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("telegram: message text must not be empty")
	}

	form := url.Values{}
	form.Set("chat_id", c.chatID)
	form.Set("text", text)

	endpoint := c.baseURL + "/bot" + c.token + "/sendMessage"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("telegram: build sendMessage request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: sendMessage: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram: read sendMessage response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("telegram: sendMessage: unexpected status %d", resp.StatusCode)
	}
	var payload sendMessageResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("telegram: parse sendMessage response: %w", err)
	}
	if !payload.OK {
		return fmt.Errorf("telegram: sendMessage rejected: %s", payload.Description)
	}
	return nil
}

func (c *Client) LLMFailure(ctx context.Context, ticker string, cause error) error {
	return c.Send(ctx, fmt.Sprintf("MOEX trader: LLM signal generation failed for %s: %v", ticker, cause))
}

func (c *Client) KillSwitchTriggered(ctx context.Context, reason string) error {
	return c.Send(ctx, fmt.Sprintf("MOEX trader: kill switch triggered: %s", reason))
}
