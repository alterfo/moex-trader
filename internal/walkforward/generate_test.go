package walkforward

import (
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestGenerateWindowSpecs_Rolling(t *testing.T) {
	from := d("2024-01-01")
	till := d("2024-07-01")
	specs, err := GenerateWindowSpecs(from, till, 90, 30, 30, false)
	if err != nil {
		t.Fatalf("GenerateWindowSpecs() error = %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("expected at least one window")
	}
	for i, s := range specs {
		wantFrom := s.Split.AddDate(0, 0, -90)
		if !s.From.Equal(wantFrom) {
			t.Fatalf("window %d: From = %v, want %v (rolling window must keep fixed train length)", i, s.From, wantFrom)
		}
		wantTill := s.Split.AddDate(0, 0, 30)
		if !s.Till.Equal(wantTill) {
			t.Fatalf("window %d: Till = %v, want %v", i, s.Till, wantTill)
		}
		if s.Till.After(till) {
			t.Fatalf("window %d: Till %v exceeds till %v", i, s.Till, till)
		}
	}
	for i := 1; i < len(specs); i++ {
		wantSplit := specs[i-1].Split.AddDate(0, 0, 30)
		if !specs[i].Split.Equal(wantSplit) {
			t.Fatalf("window %d: Split = %v, want %v (step must advance split by step-days)", i, specs[i].Split, wantSplit)
		}
	}
}

func TestGenerateWindowSpecs_Anchored(t *testing.T) {
	from := d("2024-01-01")
	till := d("2024-07-01")
	specs, err := GenerateWindowSpecs(from, till, 90, 30, 30, true)
	if err != nil {
		t.Fatalf("GenerateWindowSpecs() error = %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("expected at least one window")
	}
	for i, s := range specs {
		if !s.From.Equal(from) {
			t.Fatalf("window %d: From = %v, want anchored %v (expanding window must keep the same start)", i, s.From, from)
		}
	}
}

func TestGenerateWindowSpecs_NoWindowsWhenRangeTooShort(t *testing.T) {
	from := d("2024-01-01")
	till := d("2024-02-01")
	specs, err := GenerateWindowSpecs(from, till, 90, 30, 30, false)
	if err != nil {
		t.Fatalf("GenerateWindowSpecs() error = %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected no windows for a range shorter than train+test, got %d", len(specs))
	}
}

func TestGenerateWindowSpecs_ValidatesInputs(t *testing.T) {
	from := d("2024-01-01")
	till := d("2024-07-01")
	cases := []struct {
		name                          string
		trainDays, testDays, stepDays int
	}{
		{"zero train", 0, 30, 30},
		{"negative train", -1, 30, 30},
		{"zero test", 90, 0, 30},
		{"zero step", 90, 30, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := GenerateWindowSpecs(from, till, tc.trainDays, tc.testDays, tc.stepDays, false); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
	if _, err := GenerateWindowSpecs(till, from, 90, 30, 30, false); err == nil {
		t.Fatal("expected error when from is not before till")
	}
}

func TestGenerateWindowSpecs_LastWindowNeverExceedsTill(t *testing.T) {
	from := d("2024-01-01")
	till := d("2024-05-15")
	specs, err := GenerateWindowSpecs(from, till, 60, 20, 20, false)
	if err != nil {
		t.Fatalf("GenerateWindowSpecs() error = %v", err)
	}
	for i, s := range specs {
		if s.Till.After(till) {
			t.Fatalf("window %d: Till %v exceeds till %v", i, s.Till, till)
		}
	}
}
