package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendSuccess(t *testing.T) {
	var gotPath, gotContentType string
	var gotChatID, gotText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		gotChatID = r.FormValue("chat_id")
		gotText = r.FormValue("text")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := newClient(server.URL, "token-123", "chat-456", server.Client())
	if !client.Enabled() {
		t.Fatal("Enabled() = false, want true")
	}
	if err := client.Send(context.Background(), "hello"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if gotPath != "/bottoken-123/sendMessage" {
		t.Fatalf("request path = %q, want /bottoken-123/sendMessage", gotPath)
	}
	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Fatalf("content type = %q", gotContentType)
	}
	if gotChatID != "chat-456" {
		t.Fatalf("chat_id = %q, want chat-456", gotChatID)
	}
	if gotText != "hello" {
		t.Fatalf("text = %q, want hello", gotText)
	}
}

func TestSendAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":false,"description":"chat not found"}`)
	}))
	defer server.Close()

	client := newClient(server.URL, "token-123", "chat-456", server.Client())
	err := client.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("Send() error = nil, want API error")
	}
	if !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("Send() error = %v, want chat not found", err)
	}
}

func TestSendHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newClient(server.URL, "bad-token", "chat-456", server.Client())
	err := client.Send(context.Background(), "hello")
	if err == nil {
		t.Fatal("Send() error = nil, want HTTP error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("Send() error = %v, want status 401", err)
	}
}

func TestSendMissingConfigIsNoop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request made for disabled client")
	}))
	defer server.Close()

	tests := []struct {
		name   string
		token  string
		chatID string
	}{
		{name: "both empty", token: "", chatID: ""},
		{name: "missing token", token: "", chatID: "chat-456"},
		{name: "missing chat id", token: "token-123", chatID: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newClient(server.URL, test.token, test.chatID, server.Client())
			if client.Enabled() {
				t.Fatal("Enabled() = true, want false")
			}
			if err := client.Send(context.Background(), "hello"); err != nil {
				t.Fatalf("Send() error = %v, want nil no-op", err)
			}
		})
	}
}

func TestSemanticNotifications(t *testing.T) {
	var messages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error = %v", err)
		}
		messages = append(messages, r.FormValue("text"))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := newClient(server.URL, "token-123", "chat-456", server.Client())
	if err := client.LLMFailure(context.Background(), "SBER", fmt.Errorf("timeout")); err != nil {
		t.Fatalf("LLMFailure() error = %v", err)
	}
	if err := client.KillSwitchTriggered(context.Background(), "drawdown 3.2%%"); err != nil {
		t.Fatalf("KillSwitchTriggered() error = %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if !strings.Contains(messages[0], "SBER") || !strings.Contains(messages[0], "timeout") {
		t.Fatalf("LLM failure message = %q", messages[0])
	}
	if !strings.Contains(messages[1], "kill switch triggered") || !strings.Contains(messages[1], "drawdown 3.2%") {
		t.Fatalf("kill switch message = %q", messages[1])
	}
}
