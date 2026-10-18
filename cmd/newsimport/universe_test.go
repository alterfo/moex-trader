package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseTagsAndTickers(t *testing.T) {
	set := map[string]bool{"VKCO": true, "YDEX": true, "T": true, "SBER": true}
	tags := parseTags(`['ВК (ранее VK) VKCO', 'Яндекс YNDX', "Т-Банк TCSG", 'аналитика', 'Сбербанк SBER']`)
	if len(tags) != 5 {
		t.Fatalf("parsed %d tags: %v", len(tags), tags)
	}
	got := tagTickers(tags, set)
	want := []string{"VKCO", "YDEX", "T", "SBER"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tickers %v, want %v", got, want)
	}
}

func TestTagAliasVariants(t *testing.T) {
	cases := map[string][]string{
		"ВК (ранее VK)":          {"VK", "ВК"},
		"Каршеринг (Делимобиль)": {"Делимобиль", "Каршеринг"},
		"Сбербанк":               {"Сбербанк"},
	}
	for in, want := range cases {
		if got := tagAliasVariants(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %v, want %v", in, got, want)
		}
	}
}

func TestNormTitleIgnoresPunctuationCaseAndSpacing(t *testing.T) {
	a := normTitle("«Газпром» вырос на 3%!  ")
	b := normTitle("газпром ВЫРОС на 3")
	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
}

func TestRowTitleFallsBackToBody(t *testing.T) {
	if got := rowTitle(kasymRow{Title: "no title", Body: "  Сегежа объявила о допэмиссии  "}); got != "Сегежа объявила о допэмиссии" {
		t.Fatalf("got %q", got)
	}
	if got := rowTitle(kasymRow{Title: "Заголовок", Body: "тело"}); got != "Заголовок" {
		t.Fatalf("got %q", got)
	}
}

func TestEmitDeduplicatesByIDAndByTitle(t *testing.T) {
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "a.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	r := &universeRun{out: out, seen: map[string]bool{}, perTicker: map[string]int{}, opts: options{}}
	if err := r.emit("SBER", "id1", "Сбербанк вырос", "x", 0.5, 100); err != nil {
		t.Fatal(err)
	}
	if err := r.emit("SBER", "id1", "Сбербанк вырос", "x", 0.5, 100); err != nil {
		t.Fatal(err)
	}
	if err := r.emit("SBER", "id2", "СБЕРБАНК вырос!", "y", 0.5, 200); err != nil {
		t.Fatal(err)
	}
	if err := r.emit("GAZP", "id1", "Сбербанк вырос", "x", 0.5, 100); err != nil {
		t.Fatal(err)
	}
	if r.written != 2 || r.skipped != 2 {
		t.Fatalf("written %d skipped %d, want 2 and 2", r.written, r.skipped)
	}
}

func TestLoadBlocklistNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	if err := os.WriteFile(path, []byte(`["  Самолет ", "ТГК  1"]`), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := loadBlocklist(path)
	if err != nil {
		t.Fatal(err)
	}
	if !b["самолет"] || !b["тгк 1"] {
		t.Fatalf("unexpected blocklist %v", b)
	}
}
