package betaregime

import (
	"testing"
	"time"
)

func TestAlignDailyJoinsAndClusters(t *testing.T) {
	primary := map[time.Time]float64{
		d(2026, 1, 5): 0.01,
		d(2026, 1, 6): 0.02,
		d(2026, 2, 2): 0.03,
	}
	bench := map[time.Time]float64{
		d(2026, 1, 5): 0.10,
		d(2026, 1, 6): 0.11,
		d(2026, 2, 2): 0.12,
	}
	momentum := map[time.Time]float64{
		d(2026, 1, 5): -0.10,
		d(2026, 1, 6): -0.11,
		d(2026, 2, 2): -0.12,
	}
	windows := []Window{
		{From: d(2026, 1, 1), Till: d(2026, 1, 31)},
		{From: d(2026, 2, 1), Till: d(2026, 2, 28)},
	}
	y, x, clusters, dates, skipped := AlignDaily(primary, bench, momentum, windows)
	if skipped != 0 {
		t.Fatalf("skipped = %d, want 0", skipped)
	}
	if len(y) != 3 || len(x) != 3 || len(clusters) != 3 || len(dates) != 3 {
		t.Fatalf("lengths = %d/%d/%d/%d, want 3 each", len(y), len(x), len(clusters), len(dates))
	}
	if clusters[0] != 0 || clusters[1] != 0 || clusters[2] != 1 {
		t.Fatalf("clusters = %v, want [0 0 1]", clusters)
	}
	if !dates[0].Before(dates[1]) || !dates[1].Before(dates[2]) {
		t.Fatalf("dates not sorted: %v", dates)
	}
}

func TestAlignDailySkipsMissingAndOutOfWindow(t *testing.T) {
	primary := map[time.Time]float64{
		d(2026, 1, 5): 0.01,
		d(2026, 1, 6): 0.02,
		d(2026, 2, 2): 0.03,
		d(2026, 3, 3): 0.04,
	}
	bench := map[time.Time]float64{
		d(2026, 1, 5): 0.10,
		d(2026, 1, 6): 0.11,
		d(2026, 2, 2): 0.12,
		d(2026, 3, 3): 0.13,
	}
	momentum := map[time.Time]float64{
		d(2026, 1, 5): -0.10,
		d(2026, 1, 6): -0.11,
		d(2026, 2, 2): -0.12,
	}
	windows := []Window{{From: d(2026, 1, 1), Till: d(2026, 1, 31)}}
	y, x, _, _, skipped := AlignDaily(primary, bench, momentum, windows)
	if skipped != 2 {
		t.Fatalf("skipped = %d, want 2 (one missing series, one out of window)", skipped)
	}
	if len(y) != 2 || len(x) != 2 {
		t.Fatalf("len = %d/%d, want 2", len(y), len(x))
	}
}
