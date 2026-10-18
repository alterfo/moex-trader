package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

func fakeChannel(total int, pageSize int, start time.Time) pageFetcher {
	return func(ctx context.Context, before int64) ([]news.TelegramPost, error) {
		top := int64(total)
		if before > 0 {
			top = before - 1
		}
		var posts []news.TelegramPost
		for id := top - int64(pageSize) + 1; id <= top; id++ {
			if id < 1 {
				continue
			}
			posts = append(posts, news.TelegramPost{ID: id, PublishedAt: start.Add(time.Duration(id) * time.Hour), Text: "post"})
		}
		return posts, nil
	}
}

func collect(t *testing.T, fetch pageFetcher, b known, since time.Time, maxPages int) map[int64]int {
	t.Helper()
	got := map[int64]int{}
	emit := func(p news.TelegramPost) error {
		got[p.ID]++
		return nil
	}
	if _, err := collectChannel(context.Background(), "ch", fetch, emit, b, since, maxPages); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestCollectChannelFreshStopsAtSince(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := collect(t, fakeChannel(100, 20, start), known{}, start.Add(50*time.Hour), 0)
	if len(got) != 51 {
		t.Fatalf("collected %d posts, want 51 (ids 50..100)", len(got))
	}
	for id := int64(50); id <= 100; id++ {
		if got[id] != 1 {
			t.Fatalf("id %d seen %d times", id, got[id])
		}
	}
}

func TestCollectChannelResumeFillsOnlyMissingRanges(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := collect(t, fakeChannel(100, 20, start), known{min: 60, max: 80}, start.Add(30*time.Hour), 0)
	for id := int64(81); id <= 100; id++ {
		if got[id] != 1 {
			t.Fatalf("new id %d seen %d times", id, got[id])
		}
	}
	for id := int64(30); id < 60; id++ {
		if got[id] != 1 {
			t.Fatalf("backfill id %d seen %d times", id, got[id])
		}
	}
	for id := int64(60); id <= 80; id++ {
		if got[id] != 0 {
			t.Fatalf("already known id %d was fetched again", id)
		}
	}
}

func TestCollectChannelRespectsMaxPages(t *testing.T) {
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := collect(t, fakeChannel(100, 20, start), known{}, time.Time{}, 2)
	if len(got) != 40 {
		t.Fatalf("collected %d posts with 2 pages of 20", len(got))
	}
}

func TestWriteRecordShapeAndEmptyPostsSkipped(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	info := channels["kommersant"]
	when := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := writeRecord(w, "kommersant", info, news.TelegramPost{ID: 7, PublishedAt: when, Text: "  Сбербанк вырос  "}); err != nil {
		t.Fatal(err)
	}
	if err := writeRecord(w, "kommersant", info, news.TelegramPost{ID: 8, PublishedAt: when, Text: "   "}); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one record, got %d: %q", len(lines), buf.String())
	}
	var rec rawRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Ticker != "" || rec.Title != "Сбербанк вырос" || rec.Channel != "kommersant" || rec.PostID != 7 || rec.PublishedTS != when.Unix() || rec.Source != info.Display || rec.ArticleID == "" {
		t.Fatalf("unexpected record %+v", rec)
	}
	if len(orderedChannels) != len(channels) {
		t.Fatalf("ordered channel list (%d) must match the channel table (%d)", len(orderedChannels), len(channels))
	}
}
