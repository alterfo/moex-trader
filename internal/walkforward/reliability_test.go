package walkforward

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReliabilityCSVRoundTrip(t *testing.T) {
	rows := []ReliabilityRow{
		{Date: day(2025, 4, 1), Ticker: "SBER", Probability: 0.62, Label: 1},
		{Date: day(2025, 4, 1), Ticker: "GAZP", Probability: 0.31, Label: 0},
		{Date: day(2025, 4, 2), Ticker: "SBER", Probability: 0.55, Label: 1},
	}
	payload, err := MarshalReliabilityCSV(rows)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(payload), "date,ticker,probability,label\n") {
		t.Fatalf("reliability header missing: %q", payload)
	}
	parsed, err := ParseReliabilityCSV(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(rows) {
		t.Fatalf("len(parsed) = %d, want %d", len(parsed), len(rows))
	}
	if parsed[0].Ticker != "GAZP" || parsed[1].Ticker != "SBER" {
		t.Fatalf("rows not sorted by date then ticker: %+v", parsed)
	}
}

func TestReliabilityCSVRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"bad_header":      "date,ticker,probability,wrong\n2025-04-01,SBER,0.5,1\n",
		"bad_date":        "date,ticker,probability,label\n2025-04-xx,SBER,0.5,1\n",
		"bad_probability": "date,ticker,probability,label\n2025-04-01,SBER,1.5,1\n",
		"bad_label":       "date,ticker,probability,label\n2025-04-01,SBER,0.5,2\n",
		"duplicate":       "date,ticker,probability,label\n2025-04-01,SBER,0.5,1\n2025-04-01,SBER,0.6,0\n",
		"empty":           "",
	}
	for name, content := range cases {
		if _, err := ParseReliabilityCSV([]byte(content)); err == nil {
			t.Fatalf("%s: ParseReliabilityCSV returned no error", name)
		}
	}
}

func TestSaveLoadReliability(t *testing.T) {
	rows := []ReliabilityRow{{Date: day(2025, 4, 1), Ticker: "SBER", Probability: 0.7, Label: 1}}
	dir := t.TempDir()
	if err := SaveReliability(dir, rows); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadReliability(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Probability != 0.7 || loaded[0].Label != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestWindowValidateReliabilityConsistency(t *testing.T) {
	base := Window{ID: "w", Config: testConfig(), ConfigHash: ConfigHash(testConfig()), CreatedAt: day(2025, 4, 1)}

	fileOnly := base
	fileOnly.ReliabilityFile = ReliabilityFileName
	if err := fileOnly.Validate(); err == nil || !strings.Contains(err.Error(), "SHA256 is required") {
		t.Fatalf("Validate() error = %v, want missing-SHA256 error", err)
	}

	hashOnly := base
	hashOnly.ReliabilitySHA256 = HashBytes([]byte("x"))
	if err := hashOnly.Validate(); err == nil || !strings.Contains(err.Error(), "file is required") {
		t.Fatalf("Validate() error = %v, want missing-file error", err)
	}
}

func TestSaveLoadWindowWithReliability(t *testing.T) {
	rows := []ReliabilityRow{{Date: day(2025, 4, 1), Ticker: "SBER", Probability: 0.7, Label: 1}}
	w := Window{ID: NewWindowID(day(2025, 4, 1), day(2025, 6, 30)), Config: testConfig(), ConfigHash: ConfigHash(testConfig())}
	if err := w.SetReliabilityRows(rows); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "2025-04-01_2025-06-30")
	if err := SaveReliability(dir, rows); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, w, ""); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ReliabilityFile != ReliabilityFileName || loaded.ReliabilitySHA256 == "" {
		t.Fatalf("reliability metadata did not round-trip: %+v", loaded)
	}

	path := filepath.Join(dir, ReliabilityFileName)
	if err := os.WriteFile(path, []byte("date,ticker,probability,label\n2025-04-01,SBER,0.2,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "reliability SHA256 mismatch") {
		t.Fatalf("Load() error = %v, want reliability SHA256 mismatch", err)
	}
}
