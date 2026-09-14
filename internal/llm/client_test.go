package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatValidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.Model != "qwen3.8" {
			t.Errorf("model = %q, want qwen3.8", req.Model)
			return
		}
		if req.Format != "json" {
			t.Errorf("format = %q, want json", req.Format)
			return
		}
		if len(req.Messages) != 2 {
			t.Errorf("messages = %d, want 2", len(req.Messages))
			return
		}
		if req.Messages[0].Role != "system" || req.Messages[1].Role != "user" {
			t.Errorf("unexpected roles: %+v", req.Messages)
			return
		}
		schema, ok := req.Schema.(map[string]any)
		if !ok {
			t.Errorf("schema not present as object: %T", req.Schema)
			return
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Errorf("schema has no properties: %+v", schema)
			return
		}
		if _, ok := props["action"]; !ok {
			t.Errorf("schema properties missing action: %+v", props)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"model":"qwen3.8","message":{"role":"assistant","content":"{\"ticker\":\"SBER\",\"action\":\"BUY\",\"confidence\":0.8,\"target_lots\":1,\"reasoning\":\"up\"}"},"done":true}`)
	}))
	defer server.Close()

	client := New(server.URL, "qwen3.8", 5*time.Second)
	resp, err := client.Chat(context.Background(), []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "user prompt"},
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !resp.Done {
		t.Fatalf("Chat() done = false, want true")
	}
	if !strings.Contains(resp.Message.Content, "SBER") {
		t.Fatalf("Chat() content = %q, want trade signal JSON", resp.Message.Content)
	}
}

func TestChatMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"model":"qwen3.8","message":`)
	}))
	defer server.Close()

	client := New(server.URL, "qwen3.8", 5*time.Second)
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parse chat response") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChatTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	client := New(server.URL, "qwen3.8", 20*time.Millisecond)
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for request timeout")
	}
}

func TestChatHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := New(server.URL, "qwen3.8", 5*time.Second)
	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err == nil {
		t.Fatal("expected error for HTTP error status")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected 503 in error, got %v", err)
	}
}

func TestTradeSignalSchema(t *testing.T) {
	schema := TradeSignalSchema()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema has no properties")
	}
	action, ok := props["action"].(map[string]any)
	if !ok {
		t.Fatal("schema missing action property")
	}
	enum, ok := action["enum"].([]string)
	if !ok {
		t.Fatalf("action enum has unexpected type %T", action["enum"])
	}
	if len(enum) != 3 {
		t.Fatalf("action enum = %v, want BUY/SELL/HOLD", enum)
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("schema has no required field")
	}
	if len(required) == 0 {
		t.Fatal("schema required fields are empty")
	}
}
