package main

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

var labels = []string{"strong_negative", "negative", "neutral", "positive", "strong_positive"}

const instruction = "You classify Russian financial news headlines. Decide how the headline changes the outlook for the share price of the named company over the next days. Answer with exactly one of these words and nothing else: strong_negative, negative, neutral, positive, strong_positive."

func labelScore(label string) (float64, bool) {
	for i, l := range labels {
		if label == l {
			return float64(i - 2), true
		}
	}
	return 0, false
}

func parseLabel(reply string) (string, bool) {
	r := strings.ToLower(strings.TrimSpace(reply))
	r = strings.Trim(r, ".\"'`*")
	r = strings.ReplaceAll(strings.ReplaceAll(r, " ", "_"), "-", "_")
	for _, l := range labels {
		if r == l {
			return l, true
		}
	}
	return "", false
}

func buildPrompt(ticker, source, title string, redact bool) string {
	if redact {
		return fmt.Sprintf("Source: %s\nHeadline: %s", source, title)
	}
	return fmt.Sprintf("Ticker: %s\nSource: %s\nHeadline: %s", ticker, source, title)
}

type client struct {
	base  string
	model string
	http  *http.Client
}

func newClient(base, model string) *client {
	return &client{base: strings.TrimRight(base, "/"), model: model, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *client) ask(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":      c.model,
		"stream":     false,
		"think":      false,
		"keep_alive": "30m",
		"options":    map[string]any{"temperature": 0, "seed": 1, "num_predict": 12},
		"messages": []map[string]string{
			{"role": "system", "content": instruction},
			{"role": "user", "content": prompt},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return out.Message.Content, nil
}

func (c *client) digest(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/tags", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name   string `json:"name"`
			Digest string `json:"digest"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return "", err
	}
	for _, m := range tags.Models {
		if m.Name == c.model {
			return m.Digest, nil
		}
	}
	return "", fmt.Errorf("model %s not listed", c.model)
}
