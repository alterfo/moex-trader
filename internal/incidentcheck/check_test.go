package incidentcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseResult(t *testing.T) {
	cases := []struct {
		in      string
		want    Verdict
		wantErr bool
	}{
		{`{"verdict":"damage","reason":"пожар"}`, Damage, false},
		{` {"verdict":"No_Damage","reason":"не долетели"} `, NoDamage, false},
		{`{"verdict":"unclear"}`, Unclear, false},
		{`{"verdict":"maybe"}`, "", true},
		{`not json`, "", true},
	}
	for _, c := range cases {
		got, err := ParseResult(c.in)
		if (err != nil) != c.wantErr {
			t.Fatalf("ParseResult(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
		}
		if err == nil && got.Verdict != c.want {
			t.Fatalf("ParseResult(%q) = %q, want %q", c.in, got.Verdict, c.want)
		}
	}
}

func TestBuildPromptIncludesTitlesAndCapsThem(t *testing.T) {
	titles := make([]string, 20)
	for i := range titles {
		titles[i] = "заголовок номер " + string(rune('A'+i))
	}
	prompt := BuildPrompt("SBER", titles)
	if !strings.Contains(prompt, "SBER") || !strings.Contains(prompt, "заголовок номер A") {
		t.Fatalf("prompt misses ticker or first title: %s", prompt)
	}
	if strings.Contains(prompt, "заголовок номер I") {
		t.Fatalf("prompt must cap titles at %d", maxTitles)
	}
}

func TestOllamaCheck(t *testing.T) {
	var gotReq chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Role: "assistant", Content: `{"verdict":"no_damage","reason":"штатная работа"}`}})
	}))
	defer srv.Close()

	o := NewOllama(srv.URL, "qwen3.8:latest", 5*time.Second)
	res, err := o.Check(context.Background(), "SBER", []string{"Сбербанк сообщил о штатной работе после атаки"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != NoDamage {
		t.Fatalf("verdict = %q, want no_damage", res.Verdict)
	}
	if gotReq.Model != "qwen3.8:latest" || gotReq.Stream || gotReq.Think {
		t.Fatalf("request = %+v", gotReq)
	}
}

func TestOllamaCheckFailsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	o := NewOllama(srv.URL, "m", time.Second)
	if _, err := o.Check(context.Background(), "SBER", []string{"x"}); err == nil {
		t.Fatal("want error on 503")
	}
}
