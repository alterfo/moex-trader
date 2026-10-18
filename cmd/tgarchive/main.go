package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

const (
	maxRetries     = 4
	maxTextRunes   = 3000
	progressEvery  = 25
	flushEvery     = 100
	defaultDelay   = 1200 * time.Millisecond
	defaultOutPath = "data/news_raw_tg.jsonl"
)

type channelInfo struct {
	Display string
	Trust   float64
}

var channels = map[string]channelInfo{
	"smartlabnews":       {"Smart-lab новости (TG)", 0.25},
	"bcs_express":        {"БКС Экспресс (TG)", 0.25},
	"cbonds":             {"Cbonds (TG)", 0.25},
	"frank_media":        {"Frank Media (TG)", 0.25},
	"cbrstocks":          {"CBR Stocks (TG)", 0.25},
	"prime1":             {"Прайм (TG)", 0.25},
	"if_market_news":     {"Интерфакс Рынки (TG)", 0.25},
	"finamalert":         {"Финам (TG)", 0.25},
	"investfuture":       {"Инвестфьючерс (TG)", 0.25},
	"centralbank_russia": {"Банк России (TG)", 0.5},
	"interfaxonline":     {"Интерфакс (TG)", 0.25},
	"kommersant":         {"Коммерсантъ (TG)", 0.25},
	"rbc_news":           {"РБК (TG)", 0.25},
}

type rawRecord struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
	Channel     string  `json:"channel"`
	PostID      int64   `json:"post_id"`
}

type known struct {
	min, max int64
}

type pageFetcher func(ctx context.Context, before int64) ([]news.TelegramPost, error)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tgarchive", flag.ContinueOnError)
	configPath := fs.String("config", "config.yaml", "config path (proxy)")
	outPath := fs.String("out", defaultOutPath, "raw archive JSONL (finanalys-compatible, ticker empty)")
	sinceStr := fs.String("since", "2025-01-01", "collect posts back to YYYY-MM-DD")
	chanList := fs.String("channels", "", "comma-separated channels (default: all known)")
	delay := fs.Duration("delay", defaultDelay, "pause between page requests")
	maxPages := fs.Int("max-pages", 0, "max pages per channel and phase (0 = unlimited)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	since, err := time.Parse("2006-01-02", *sinceStr)
	if err != nil {
		return fmt.Errorf("parse -since: %w", err)
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	proxy := cfg.News.Proxy
	if proxy == "" {
		proxy = cfg.Telegram.Proxy
	}
	fetcher := news.NewFetcher(newHTTPClient(proxy))

	names := channelNames(*chanList)
	for _, ch := range names {
		if _, ok := channels[ch]; !ok {
			return fmt.Errorf("unknown channel %q", ch)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	bounds, err := loadKnown(*outPath)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(*outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	w := bufio.NewWriter(out)

	for _, ch := range names {
		info := channels[ch]
		fetch := func(ctx context.Context, before int64) ([]news.TelegramPost, error) {
			return fetchWithRetry(ctx, fetcher, ch, before, *delay)
		}
		emitted := 0
		emit := func(p news.TelegramPost) error {
			emitted++
			if emitted%flushEvery == 0 {
				if err := w.Flush(); err != nil {
					return err
				}
			}
			return writeRecord(w, ch, info, p)
		}
		b := bounds[ch]
		n, err := collectChannel(ctx, ch, fetch, emit, b, since, *maxPages)
		if flushErr := w.Flush(); flushErr != nil {
			return flushErr
		}
		log.Printf("tgarchive: %s: %d new posts", ch, n)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			log.Printf("tgarchive: %s stopped: %v", ch, err)
		}
	}
	return nil
}

func channelNames(list string) []string {
	if strings.TrimSpace(list) == "" {
		names := make([]string, 0, len(channels))
		for _, ch := range orderedChannels {
			names = append(names, ch)
		}
		return names
	}
	var out []string
	for _, ch := range strings.Split(list, ",") {
		if ch = strings.TrimSpace(ch); ch != "" {
			out = append(out, ch)
		}
	}
	return out
}

var orderedChannels = []string{"smartlabnews", "bcs_express", "cbonds", "frank_media", "cbrstocks", "prime1", "if_market_news", "finamalert", "investfuture", "centralbank_russia", "interfaxonline", "kommersant", "rbc_news"}

func writeRecord(w *bufio.Writer, ch string, info channelInfo, p news.TelegramPost) error {
	text := strings.TrimSpace(p.Text)
	if text == "" {
		return nil
	}
	rs := []rune(text)
	if len(rs) > maxTextRunes {
		text = string(rs[:maxTextRunes])
	}
	link := fmt.Sprintf("https://t.me/%s/%d", ch, p.ID)
	raw, err := json.Marshal(rawRecord{
		Ticker: "", ArticleID: news.ArticleID(link, text), Title: text, Source: info.Display,
		TrustWeight: info.Trust, PublishedTS: p.PublishedAt.Unix(), Channel: ch, PostID: p.ID,
	})
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

func collectChannel(ctx context.Context, ch string, fetch pageFetcher, emit func(news.TelegramPost) error, b known, since time.Time, maxPages int) (int, error) {
	written := 0
	phase := func(start int64, stopAtOrBelow int64) error {
		before := start
		for page := 0; maxPages <= 0 || page < maxPages; page++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			posts, err := fetch(ctx, before)
			if err != nil {
				return err
			}
			if len(posts) == 0 {
				return nil
			}
			minID := posts[0].ID
			done := false
			for _, p := range posts {
				if p.ID < minID {
					minID = p.ID
				}
				if !since.IsZero() && !p.PublishedAt.IsZero() && p.PublishedAt.Before(since) {
					done = true
					continue
				}
				if stopAtOrBelow > 0 && p.ID <= stopAtOrBelow {
					done = true
					continue
				}
				if err := emit(p); err != nil {
					return err
				}
				written++
			}
			if page%progressEvery == progressEvery-1 {
				log.Printf("tgarchive: %s page %d, oldest id %d, %d written", ch, page+1, minID, written)
			}
			if done || minID == before {
				return nil
			}
			before = minID
		}
		return nil
	}
	if b.max > 0 {
		if err := phase(0, b.max); err != nil {
			return written, err
		}
		if b.min > 1 {
			if err := phase(b.min, 0); err != nil {
				return written, err
			}
		}
		return written, nil
	}
	return written, phase(0, 0)
}

func fetchWithRetry(ctx context.Context, f *news.Fetcher, ch string, before int64, delay time.Duration) ([]news.TelegramPost, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay * time.Duration(1<<attempt)):
		}
		posts, err := f.FetchTelegramPage(ctx, ch, before)
		if err == nil {
			return posts, nil
		}
		lastErr = err
		var status *news.TelegramStatusError
		if errors.As(err, &status) && status.Status != http.StatusTooManyRequests && status.Status < 500 {
			return nil, err
		}
	}
	return nil, lastErr
}

func loadKnown(path string) (map[string]known, error) {
	out := map[string]known{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var rec struct {
			Channel string `json:"channel"`
			PostID  int64  `json:"post_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &rec) != nil || rec.Channel == "" || rec.PostID <= 0 {
			continue
		}
		k := out[rec.Channel]
		if k.min == 0 || rec.PostID < k.min {
			k.min = rec.PostID
		}
		if rec.PostID > k.max {
			k.max = rec.PostID
		}
		out[rec.Channel] = k
	}
	return out, scanner.Err()
}

func newHTTPClient(proxy string) *http.Client {
	if proxy == "" {
		return nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		log.Printf("tgarchive: invalid proxy %q: %v (falling back to direct)", proxy, err)
		return nil
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
}
