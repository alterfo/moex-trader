package incidentcheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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
	Verdict   Verdict
	Reason    string
	Raw       string
	Model     string
	Digest    string
	PromptSHA string
}

type Checker interface {
	Check(ctx context.Context, ticker string, titles []string) (Result, error)
}

type Ollama struct {
	baseURL   string
	model     string
	client    *http.Client
	digestPin string

	mu     sync.Mutex
	digest string
}

func (o *Ollama) WithDigestPin(digest string) *Ollama {
	o.digestPin = strings.TrimSpace(digest)
	return o
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

type tagsResponse struct {
	Models []struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	} `json:"models"`
}

func (o *Ollama) modelDigest(ctx context.Context) string {
	o.mu.Lock()
	cached := o.digest
	o.mu.Unlock()
	if cached != "" {
		return cached
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return ""
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var tags tagsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&tags); err != nil {
		return ""
	}
	for _, m := range tags.Models {
		if m.Name == o.model {
			o.mu.Lock()
			o.digest = m.Digest
			o.mu.Unlock()
			return m.Digest
		}
	}
	return ""
}

func promptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])[:16]
}

func (o *Ollama) Check(ctx context.Context, ticker string, titles []string) (Result, error) {
	prompt := BuildPrompt(ticker, titles)
	meta := Result{Model: o.model, PromptSHA: promptHash(prompt)}
	meta.Digest = o.modelDigest(ctx)
	if o.digestPin != "" && meta.Digest != o.digestPin {
		return meta, fmt.Errorf("incident check: model %q digest %q does not match the pinned %q", o.model, meta.Digest, o.digestPin)
	}
	body, err := json.Marshal(chatRequest{
		Model:    o.model,
		Messages: []chatMessage{{Role: "user", Content: prompt}},
		Stream:   false,
		Format:   "json",
		Think:    false,
		Options:  map[string]any{"temperature": 0, "num_predict": 200},
	})
	if err != nil {
		return meta, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return meta, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return meta, fmt.Errorf("incident check: request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return meta, fmt.Errorf("incident check: read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return meta, fmt.Errorf("incident check: status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed chatResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return meta, fmt.Errorf("incident check: decode: %w", err)
	}
	meta.Raw = parsed.Message.Content
	result, err := ParseResult(parsed.Message.Content)
	if err != nil {
		return meta, err
	}
	result.Raw, result.Model, result.Digest, result.PromptSHA = meta.Raw, meta.Model, meta.Digest, meta.PromptSHA
	return result, nil
}
