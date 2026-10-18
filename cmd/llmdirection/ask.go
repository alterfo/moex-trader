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

type asker struct {
	baseURL string
	model   string
	client  *http.Client
}

func newAsker(baseURL, model string, timeout time.Duration) *asker {
	return &asker{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client:  &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}},
	}
}

func buildPrompt(ticker, title string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ты аналитик российского фондового рынка. Заголовок новости о компании %s:\n", ticker)
	fmt.Fprintf(&b, "«%s»\n\n", strings.TrimSpace(title))
	fmt.Fprintf(&b, "Как эта новость скорее повлияет на цену акции %s относительно индекса МосБиржи в ближайшие дни?\n", ticker)
	b.WriteString("up: акция вырастет сильнее индекса. down: акция вырастет слабее индекса или упадёт сильнее. neutral: направление неясно или влияния нет.\n")
	b.WriteString(`Ответь только JSON: {"direction":"up|down|neutral","confidence":1-5}`)
	return b.String()
}

type answer struct {
	Direction  string  `json:"direction"`
	Confidence float64 `json:"confidence"`
}

func parseAnswer(content string) (float64, string, error) {
	var a answer
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &a); err != nil {
		return 0, "", fmt.Errorf("parse %q: %w", content, err)
	}
	conf := a.Confidence
	if conf < 1 {
		conf = 1
	}
	if conf > 5 {
		conf = 5
	}
	switch strings.ToLower(strings.TrimSpace(a.Direction)) {
	case "up":
		return conf, "up", nil
	case "down":
		return -conf, "down", nil
	case "neutral":
		return 0, "neutral", nil
	}
	return 0, "", fmt.Errorf("unknown direction %q", a.Direction)
}

func (a *asker) ask(ctx context.Context, ticker, title string) (float64, string, string, error) {
	body, err := json.Marshal(map[string]any{
		"model":    a.model,
		"messages": []map[string]string{{"role": "user", "content": buildPrompt(ticker, title)}},
		"stream":   false,
		"format":   "json",
		"think":    false,
		"options":  map[string]any{"temperature": 0, "num_predict": 60},
	})
	if err != nil {
		return 0, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return 0, "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, "", "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, "", "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, "", "", err
	}
	score, dir, perr := parseAnswer(out.Message.Content)
	if perr != nil {
		return 0, "", out.Message.Content, perr
	}
	return score, dir, out.Message.Content, nil
}
