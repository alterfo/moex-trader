package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

type record struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
}

func main() {
	in := flag.String("in", "data/news_history.jsonl", "news archive to re-attribute")
	out := flag.String("out", "data/news_history.rematched.jsonl", "output archive")
	industry := flag.Bool("industry", false, "also emit industry-derived (indirect) matches at half trust")
	flag.Parse()

	if err := run(*in, *out, *industry); err != nil {
		log.Fatal(err)
	}
}

func run(inPath, outPath string, industry bool) error {
	src, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	matcher := news.NewMatcher(news.DefaultAliases())
	if industry {
		matcher = matcher.WithIndustryRules(news.DefaultIndustryRules())
	}

	writer := bufio.NewWriter(dst)
	defer writer.Flush()
	encoder := json.NewEncoder(writer)

	seen := make(map[string]struct{})
	var kept, rematched, dropped int
	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<24)
	for scanner.Scan() {
		var rec record
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if strings.HasPrefix(rec.Source, "Google News:") {
			key := rec.ArticleID + "|" + rec.Ticker
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if err := encoder.Encode(rec); err != nil {
				return err
			}
			kept++
			continue
		}
		article := news.Article{
			Title:       rec.Title,
			Link:        rec.ArticleID,
			PublishedAt: time.Unix(rec.PublishedTS, 0),
			Source:      news.Source{Name: rec.Source, TrustWeight: decimal.NewFromFloat(rec.TrustWeight)},
		}
		matches := matcher.Match([]news.Article{article})
		if len(matches) == 0 {
			dropped++
			continue
		}
		for _, match := range matches {
			key := rec.ArticleID + "|" + match.Ticker
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			weight, _ := match.TrustWeight.Float64()
			next := rec
			next.Ticker = match.Ticker
			next.TrustWeight = weight
			if err := encoder.Encode(next); err != nil {
				return err
			}
			rematched++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Printf("google kept=%d rematched=%d dropped(no ticker)=%d\n", kept, rematched, dropped)
	return nil
}
