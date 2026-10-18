package incidentcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestOllamaCheckCapturesProvenanceAndPinsDigest(t *testing.T) {
	chatCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"other:1","digest":"sha256:zzz"},{"name":"m:latest","digest":"sha256:abc123"}]}`))
		case "/api/chat":
			chatCalls++
			_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Role: "assistant", Content: `{"verdict":"damage","reason":"пожар"}`}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	res, err := NewOllama(srv.URL, "m:latest", time.Second).Check(context.Background(), "SBER", []string{"пожар на объекте"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != Damage || res.Model != "m:latest" || res.Digest != "sha256:abc123" || len(res.PromptSHA) != 16 || res.Raw != `{"verdict":"damage","reason":"пожар"}` {
		t.Fatalf("missing provenance in %+v", res)
	}

	pinned := NewOllama(srv.URL, "m:latest", time.Second).WithDigestPin("sha256:abc123")
	if _, err := pinned.Check(context.Background(), "SBER", []string{"x"}); err != nil {
		t.Fatalf("matching pin must pass: %v", err)
	}
	callsBefore := chatCalls
	wrong := NewOllama(srv.URL, "m:latest", time.Second).WithDigestPin("sha256:other")
	got, err := wrong.Check(context.Background(), "SBER", []string{"x"})
	if err == nil || chatCalls != callsBefore {
		t.Fatalf("a digest mismatch must fail before any chat call: err=%v calls %d -> %d", err, callsBefore, chatCalls)
	}
	if got.Digest != "sha256:abc123" || got.Model != "m:latest" {
		t.Fatalf("the error result must still carry provenance: %+v", got)
	}
}

func TestParseFailureKeepsRawAnswerForAudit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chat" {
			_ = json.NewEncoder(w).Encode(chatResponse{Message: chatMessage{Role: "assistant", Content: "это не json"}})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	res, err := NewOllama(srv.URL, "m", time.Second).Check(context.Background(), "SBER", []string{"x"})
	if err == nil || res.Raw != "это не json" {
		t.Fatalf("raw answer must survive a parse failure: res=%+v err=%v", res, err)
	}
}

func TestAuditLogAppendsJSONLinesAndNilIsNoop(t *testing.T) {
	var nilLog *AuditLog
	if err := nilLog.Append(AuditRecord{}); err != nil {
		t.Fatal(err)
	}
	if NewAuditLog("") != nil {
		t.Fatal("empty path must disable the log")
	}
	path := t.TempDir() + "/sub/audit.jsonl"
	log := NewAuditLog(path)
	for i := 0; i < 3; i++ {
		if err := log.Append(AuditRecord{Ticker: "SBER", Outcome: "rejected", Titles: []string{"t"}}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil || strings.Count(string(raw), "\n") != 3 {
		t.Fatalf("expected 3 lines, got %q (err %v)", raw, err)
	}
}
