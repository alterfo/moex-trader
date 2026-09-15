package main

import "testing"

func TestParseOptionsDefaults(t *testing.T) {
	opts, err := parseOptions(nil)
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.configPath != defaultConfigPath || opts.days != defaultDays || opts.maxLag != defaultMaxLag ||
		opts.grangerLag != defaultGrangerLag || opts.minOverlap != defaultMinOverlap ||
		opts.minAsymmetry != defaultMinAsymmetry || opts.top != defaultTop {
		t.Fatalf("unexpected defaults: %+v", opts)
	}
}

func TestParseOptionsOverrides(t *testing.T) {
	opts, err := parseOptions([]string{
		"-config", "other.yaml",
		"-targets", "SBER,GAZP",
		"-candidates", "IMOEX",
		"-days", "365",
		"-max-lag", "3",
		"-granger-lag", "1",
		"-min-overlap", "30",
		"-min-asymmetry", "0.1",
		"-top", "5",
		"-out", "/tmp/leadlag.md",
	})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.targetsFlag != "SBER,GAZP" || opts.candidatesFlag != "IMOEX" || opts.days != 365 ||
		opts.maxLag != 3 || opts.grangerLag != 1 || opts.minOverlap != 30 || opts.minAsymmetry != 0.1 ||
		opts.top != 5 || opts.outPath != "/tmp/leadlag.md" {
		t.Fatalf("unexpected options: %+v", opts)
	}
}

func TestParseOptionsRobustnessRequiresCandidateAndTarget(t *testing.T) {
	if _, err := parseOptions([]string{"-robustness"}); err == nil {
		t.Fatal("parseOptions() error = nil, want error when -robustness is set without -candidate/-target")
	}
	if _, err := parseOptions([]string{"-robustness", "-candidate", "MTSS"}); err == nil {
		t.Fatal("parseOptions() error = nil, want error when -target is missing")
	}
	opts, err := parseOptions([]string{"-robustness", "-candidate", "MTSS", "-target", "SBER"})
	if err != nil {
		t.Fatalf("parseOptions() error = %v", err)
	}
	if opts.candidateFlag != "MTSS" || opts.targetFlag != "SBER" || opts.leadDays != defaultLeadDays || opts.windowDays != defaultWindowDays {
		t.Fatalf("unexpected robustness options: %+v", opts)
	}
}

func TestParseOptionsRejectsPositionalArgs(t *testing.T) {
	if _, err := parseOptions([]string{"unexpected"}); err == nil {
		t.Fatal("parseOptions() error = nil, want unexpected argument error")
	}
}

func TestUnionStringsDedupsPreservingOrder(t *testing.T) {
	got := unionStrings([]string{"A", "B"}, []string{"B", "C"})
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("unionStrings() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unionStrings() = %v, want %v", got, want)
		}
	}
}

func TestSplitCommaTrimsAndDropsEmpty(t *testing.T) {
	got := splitComma(" SBER, GAZP ,,  ")
	want := []string{"SBER", "GAZP"}
	if len(got) != len(want) {
		t.Fatalf("splitComma() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitComma() = %v, want %v", got, want)
		}
	}
}
