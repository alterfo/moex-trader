package main

import "testing"

func TestParseLabel(t *testing.T) {
	cases := map[string]string{"Positive": "positive", " strong_negative.": "strong_negative", "strong positive": "strong_positive", "**neutral**": "neutral"}
	for in, want := range cases {
		got, ok := parseLabel(in)
		if !ok || got != want {
			t.Fatalf("%q: got %q %v", in, got, ok)
		}
	}
	if _, ok := parseLabel("maybe positive"); ok {
		t.Fatal("unparseable accepted")
	}
	if v, _ := labelScore("strong_negative"); v != -2 {
		t.Fatal(v)
	}
}

func TestBuildPromptRedact(t *testing.T) {
	if p := buildPrompt("SBER", "RBC", "x", true); p != "Source: RBC\nHeadline: x" {
		t.Fatal(p)
	}
}
