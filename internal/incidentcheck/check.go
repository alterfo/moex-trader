package incidentcheck

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

type Verdict string

const (
	Damage   Verdict = "damage"
	NoDamage Verdict = "no_damage"
	Unclear  Verdict = "unclear"
)

const maxTitles = 8

type Result struct {
	Verdict Verdict
	Reason  string
}

type Checker interface {
	Check(ctx context.Context, ticker string, titles []string) (Result, error)
}

type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllama(baseURL, model string, timeout time.Duration) *Ollama {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Ollama{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   model,
		client: &http.Client{
			Timeout:   timeout,
			Transport: &http.Transport{Proxy: nil},
		},
	}
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Format   string         `json:"format"`
	Think    bool           `json:"think"`
	Options  map[string]any `json:"options"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
}

func BuildPrompt(ticker string, titles []string) string {
	if len(titles) > maxTitles {
		titles = titles[:maxTitles]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Ты помогаешь трейдеру оценить новость об инциденте (атака, пожар, остановка работы) для компании %s.\n", ticker)
	b.WriteString("Заголовки:\n")
	for i, title := range titles {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.TrimSpace(title))
	}
	b.WriteString("\nВопрос: инцидент реально нанёс ущерб бизнесу компании или её объекту настолько, что это может повлиять на цену акций?\n")
	b.WriteString("damage: объект поражён, пожар, разрушения, остановка работы или сбои подтверждены.\n")
	b.WriteString("no_damage: беспилотники не долетели или сбиты, ущерба нет, компания сообщает о штатной работе, речь о угрозе или о другом объекте.\n")
	b.WriteString("unclear: по заголовкам нельзя понять, был ли ущерб.\n")
	b.WriteString("Если есть и сообщения об атаке, и сообщения о штатной работе без последствий, выбирай no_damage.\n")
	b.WriteString(`Ответь только JSON: {"verdict":"damage|no_damage|unclear","reason":"одна короткая фраза"}`)
	return b.String()
}

func ParseResult(content string) (Result, error) {
	var raw struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &raw); err != nil {
		return Result{}, fmt.Errorf("incident check: parse answer %q: %w", content, err)
	}
	switch Verdict(strings.ToLower(strings.TrimSpace(raw.Verdict))) {
	case Damage:
		return Result{Verdict: Damage, Reason: raw.Reason}, nil
	case NoDamage:
		return Result{Verdict: NoDamage, Reason: raw.Reason}, nil
	case Unclear:
		return Result{Verdict: Unclear, Reason: raw.Reason}, nil
	}
	return Result{}, fmt.Errorf("incident check: unknown verdict %q", raw.Verdict)
}

func (o *Ollama) Check(ctx context.Context, ticker string, titles []string) (Result, error) {
	body, err := json.Marshal(chatRequest{
		Model:    o.model,
		Messages: []chatMessage{{Role: "user", Content: BuildPrompt(ticker, titles)}},
		Stream:   false,
		Format:   "json",
		Think:    false,
		Options:  map[string]any{"temperature": 0, "num_predict": 200},
	})
	if err != nil {
		return Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("incident check: request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, fmt.Errorf("incident check: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("incident check: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{}, fmt.Errorf("incident check: decode: %w", err)
	}
	return ParseResult(parsed.Message.Content)
}
