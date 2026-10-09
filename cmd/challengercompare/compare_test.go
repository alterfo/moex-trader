package main

import (
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func series(start string, closes ...float64) closeSeries {
	t, _ := time.Parse("2006-01-02", start)
	s := closeSeries{closes: closes}
	for i := range closes {
		s.dates = append(s.dates, t.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return s
}

func rec(day string, ticker, champion, challenger string) record {
	t, _ := time.Parse("2006-01-02 15:04", day+" 12:00")
	return record{Time: t, Ticker: ticker, Champion: side{Action: champion}, Challenger: side{Action: challenger}}
}

func TestBuildPairsUsesLastRecordPerDayAndForwardReturn(t *testing.T) {
	loc := time.UTC
	s := map[string]closeSeries{"SBER": series("2026-10-01", 100, 101, 102, 110)}
	records := []record{
		rec("2026-10-01", "SBER", "HOLD", "HOLD"),
		rec("2026-10-01", "SBER", "BUY", "SELL"),
		rec("2026-10-03", "SBER", "BUY", "BUY"),
	}
	records[1].Time = records[1].Time.Add(time.Hour)
	pairs, skipped, errs := buildPairs(records, s, 2, loc)
	if len(pairs) != 1 || skipped != 1 || errs != 0 {
		t.Fatalf("pairs=%v skipped=%d errs=%d", pairs, skipped, errs)
	}
	if pairs[0].champion != 1 || pairs[0].challenger != -1 || math.Abs(pairs[0].forward-0.02) > 1e-12 {
		t.Fatalf("pair = %+v", pairs[0])
	}
}

func TestBuildPairsCountsChallengerErrors(t *testing.T) {
	s := map[string]closeSeries{"SBER": series("2026-10-01", 100, 101, 102)}
	r := rec("2026-10-01", "SBER", "BUY", "")
	r.Challenger.Error = "bad"
	pairs, _, errs := buildPairs([]record{r}, s, 1, time.UTC)
	if len(pairs) != 0 || errs != 1 {
		t.Fatalf("pairs=%v errs=%d", pairs, errs)
	}
}

func TestSummarizeDiffAndVerdict(t *testing.T) {
	var pairs []pair
	for i := 0; i < 60; i++ {
		pairs = append(pairs, pair{date: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), champion: -1, challenger: 1, forward: 0.01})
	}
	s := summarize(pairs, 0, 0, 500, rand.New(rand.NewSource(1)))
	if math.Abs(s.DiffMean-0.02) > 1e-12 || s.Agreement != 0 || s.ChallengerHit != 1 || s.ChampionHit != 0 {
		t.Fatalf("summary = %+v", s)
	}
	if got := verdict(s, 50, 50); !strings.HasPrefix(got, "challenger better") {
		t.Fatalf("verdict = %q", got)
	}
	if got := verdict(s, 500, 50); !strings.HasPrefix(got, "inconclusive: not enough") {
		t.Fatalf("verdict = %q", got)
	}
}
