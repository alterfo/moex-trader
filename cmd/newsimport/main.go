package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	parquet "github.com/parquet-go/parquet-go"

	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

const (
	defaultTrustWeight = 0.5
)

type options struct {
	configPath  string
	outPath     string
	parquets    []string
	trustWeight float64
	tickers     string
}

type kasymRow struct {
	Title  string `parquet:"title"`
	Body   string `parquet:"body"`
	Date   string `parquet:"date"`
	Time   string `parquet:"time"`
	Tags   string `parquet:"tags"`
	Source string `parquet:"source"`
}

type record struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
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
	if len(opts.parquets) == 0 {
		return fmt.Errorf("at least one -parquet path is required")
	}

	aliases := news.DefaultAliases()
	aliasToTickers := reverseAliases(aliases)

	out, err := os.OpenFile(opts.outPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open archive %q: %w", opts.outPath, err)
	}
	defer out.Close()

	seen, err := loadArticleIDs(opts.outPath)
	if err != nil {
		return fmt.Errorf("scan archive: %w", err)
	}

	var totalNew int
	for _, path := range opts.parquets {
		n, err := importParquet(path, out, seen, aliasToTickers, opts)
		if err != nil {
			return err
		}
		totalNew += n
		log.Printf("newsimport: %s: %d new records", path, n)
	}
	log.Printf("newsimport: %d new records appended to %s", totalNew, opts.outPath)
	return nil
}

func importParquet(path string, out *os.File, seen map[string]bool, aliasToTickers map[string][]string, opts options) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open parquet %q: %w", path, err)
	}
	defer f.Close()

	pr := parquet.NewGenericReader[kasymRow](f)
	defer pr.Close()

	var newRecords int
	rows := make([]kasymRow, 1000)
	for {
		n, err := pr.Read(rows)
		if err != nil {
			if err == io.EOF {
				break
			}
			return newRecords, fmt.Errorf("read parquet %q rows: %w", path, err)
		}
		if n == 0 {
			break
		}
		for _, row := range rows[:n] {
			published, ok := parsePublished(row.Date, row.Time)
			if !ok {
				continue
			}
			text := strings.ToLower(row.Tags + " " + row.Title)
			tickers := matchTickers(text, aliasToTickers)
			if len(tickers) == 0 {
				continue
			}
			id := articleID(row.Title, row.Body)
			for _, ticker := range tickers {
				if !wanted(ticker, opts.tickers) {
					continue
				}
				key := ticker + "|" + id
				if seen[key] {
					continue
				}
				seen[key] = true
				rec := record{
					Ticker:      ticker,
					ArticleID:   id,
					Title:       row.Title,
					Source:      row.Source,
					TrustWeight: opts.trustWeight,
					PublishedTS: published.Unix(),
				}
				line, err := json.Marshal(rec)
				if err != nil {
					return newRecords, err
				}
				if _, err := out.Write(append(line, '\n')); err != nil {
					return newRecords, fmt.Errorf("write archive: %w", err)
				}
				newRecords++
			}
		}
	}
	return newRecords, nil
}

func reverseAliases(aliases map[string][]string) map[string][]string {
	out := make(map[string][]string)
	for ticker, alts := range aliases {
		for _, alt := range alts {
			key := strings.ToLower(strings.TrimSpace(alt))
			if key == "" {
				continue
			}
			out[key] = append(out[key], ticker)
		}
	}
	return out
}

// matchTickers finds tickers whose company alias appears in the tags/title.
// To avoid substring noise ("вк" inside "ВКЛЮЧАЕТ", "биржи" inside "обслуживания"),
// single-word aliases must match on word boundaries; multi-word aliases must not be
// embedded inside a longer word run but plain containment is fine for them.
func matchTickers(text string, aliasToTickers map[string][]string) []string {
	var hits []string
	matched := map[string]bool{}
	for alias, tickers := range aliasToTickers {
		if !containsOnBoundary(text, alias) {
			continue
		}
		for _, t := range tickers {
			if !matched[t] {
				matched[t] = true
				hits = append(hits, t)
			}
		}
	}
	return hits
}

func containsOnBoundary(text, alias string) bool {
	idx := strings.Index(text, alias)
	for idx >= 0 {
		beforeOK := idx == 0 || !isWordRune(rune(text[idx-1]))
		after := idx + len(alias)
		afterOK := after >= len(text) || !isWordRune(rune(text[after]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(text[after:], alias)
		if next < 0 {
			return false
		}
		idx = after + next
	}
	return false
}

func isWordRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || (r >= '0' && r <= '9')
}

func parsePublished(date, t string) (time.Time, bool) {
	d := strings.TrimSpace(date)
	if d == "" || d == "not stated" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", d[:10])
	if err != nil {
		return time.Time{}, false
	}
	hm := strings.TrimSpace(t)
	if hm != "" && hm != "not stated" {
		if tm, err := time.Parse("15:04:05", hm); err == nil {
			parsed = parsed.Add(time.Duration(tm.Hour())*time.Hour + time.Duration(tm.Minute())*time.Minute + time.Duration(tm.Second())*time.Second)
		}
	}
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.UTC), true
}

func articleID(title, body string) string {
	sum := sha256.Sum256([]byte(title + "|" + body))
	return hex.EncodeToString(sum[:])[:16]
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

func loadArticleIDs(path string) (map[string]bool, error) {
	seen := make(map[string]bool)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return seen, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		seen[rec.Ticker+"|"+rec.ArticleID] = true
	}
	return seen, scanner.Err()
}

func parseOptions(args []string) (options, error) {
	opts := options{configPath: "config.yaml", trustWeight: defaultTrustWeight}
	fs := flag.NewFlagSet("newsimport", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "config path")
	fs.StringVar(&opts.outPath, "out", "", "archive JSONL path (default: config news.raw_path or news_history.jsonl)")
	fs.Float64Var(&opts.trustWeight, "trust-weight", opts.trustWeight, "trust weight to assign kasymkhan articles")
	fs.StringVar(&opts.tickers, "tickers", "", "comma-separated tickers to keep (default: all)")
	fs.Func("parquet", "kasymkhan parquet path (repeatable)", func(value string) error {
		opts.parquets = append(opts.parquets, value)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}