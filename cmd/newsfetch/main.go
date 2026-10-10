package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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

type options struct {
	configPath string
	outPath    string
	sinceDays  int
	retroFrom  string
	maxPages   int
	tickers    string
	mergeSpool string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if opts.outPath == "" {
		opts.outPath = cfg.News.RawPath
	}
	if opts.outPath == "" {
		opts.outPath = "news_history.jsonl"
	}

	if opts.mergeSpool != "" {
		merged, skipped, err := news.MergeSpool(opts.mergeSpool, opts.outPath)
		if err != nil {
			return fmt.Errorf("merge spool: %w", err)
		}
		log.Printf("newsfetch: merged %d spool records into %s (%d duplicates)", merged, opts.outPath, skipped)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	since := time.Now().Add(-time.Duration(opts.sinceDays) * 24 * time.Hour)
	if opts.retroFrom != "" {
		retroFrom, err := time.Parse("2006-01-02", opts.retroFrom)
		if err != nil {
			return fmt.Errorf("parse -retro-from: %w", err)
		}
		since = retroFrom
	}

	out, err := os.OpenFile(opts.outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open archive %q: %w", opts.outPath, err)
	}
	defer out.Close()

	seen, err := loadArticleIDs(opts.outPath)
	if err != nil {
		return fmt.Errorf("scan archive: %w", err)
	}

	proxy := cfg.News.Proxy
	if proxy == "" {
		proxy = cfg.Telegram.Proxy
	}
	httpClient := newHTTPClient(proxy)
	fetcher := news.NewFetcher(httpClient)
	matcher := news.NewMatcher(news.DefaultAliases())

	sources := news.AllSources(cfg.Tickers)
	if opts.retroFrom != "" {
		sources = telegramOnly(sources)
	}

	var totalNew int
	for _, source := range sources {
		items, err := fetchSource(ctx, fetcher, source, since, opts.maxPages)
		if err != nil {
			log.Printf("newsfetch: %s: %v", source.Name, err)
			continue
		}
		if len(items) == 0 {
			continue
		}
		matches := matcher.Match(items)
		for _, m := range matches {
			if m.PublishedAt.Before(since) {
				continue
			}
			if !wanted(m.Ticker, opts.tickers) {
				continue
			}
			id := news.ArticleID(m.Link, m.Title)
			key := m.Ticker + "|" + id
			if seen[key] {
				continue
			}
			seen[key] = true
			rec := news.ArchiveRecord{
				Ticker:      m.Ticker,
				ArticleID:   id,
				Title:       m.Title,
				Source:      m.SourceName,
				TrustWeight: m.TrustWeight.InexactFloat64(),
				PublishedTS: m.PublishedAt.Unix(),
			}
			line, err := json.Marshal(rec)
			if err != nil {
				return fmt.Errorf("marshal record: %w", err)
			}
			if _, err := out.Write(append(line, '\n')); err != nil {
				return fmt.Errorf("write archive: %w", err)
			}
			totalNew++
		}
	}
	log.Printf("newsfetch: %d new records appended to %s", totalNew, opts.outPath)
	return nil
}

func fetchSource(ctx context.Context, fetcher *news.Fetcher, source news.Source, since time.Time, maxPages int) ([]news.Article, error) {
	if source.Type == "telegram" {
		return fetcher.FetchTelegram(ctx, source, since, maxPages)
	}
	return fetcher.Fetch(ctx, source)
}

func telegramOnly(sources []news.Source) []news.Source {
	out := make([]news.Source, 0, len(sources))
	for _, s := range sources {
		if s.Type == "telegram" {
			out = append(out, s)
		}
	}
	return out
}

func wanted(ticker, pickers string) bool {
	if strings.TrimSpace(pickers) == "" {
		return true
	}
	for _, t := range strings.Split(pickers, ",") {
		if strings.TrimSpace(t) == ticker {
			return true
		}
	}
	return false
}

func parseOptions(args []string) (options, error) {
	opts := options{configPath: "config.yaml", sinceDays: 1, maxPages: 2000}
	fs := flag.NewFlagSet("newsfetch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "config path")
	fs.StringVar(&opts.outPath, "out", "", "archive JSONL path (default: config news.raw_path or news_history.jsonl)")
	fs.IntVar(&opts.sinceDays, "since-days", opts.sinceDays, "collect news from last N days")
	fs.StringVar(&opts.retroFrom, "retro-from", "", "backfill telegram history back to YYYY-MM-DD")
	fs.IntVar(&opts.maxPages, "max-pages", opts.maxPages, "max telegram pages per channel (retro backfill)")
	fs.StringVar(&opts.mergeSpool, "merge-spool", "", "merge this spool JSONL into the archive (and empty it) before fetching")
	fs.StringVar(&opts.tickers, "tickers", "", "comma-separated tickers to keep (default: all)")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func newHTTPClient(proxy string) *http.Client {
	if proxy == "" {
		return nil
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		log.Printf("newsfetch: invalid proxy %q: %v (falling back to direct)", proxy, err)
		return nil
	}
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
}

func loadArticleIDs(path string) (map[string]bool, error) {
	records, err := news.ReadRecords(path)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(records))
	for _, rec := range records {
		seen[news.RecordKey(rec)] = true
	}
	return seen, nil
}
