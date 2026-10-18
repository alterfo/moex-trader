package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

type rawArticle struct {
	Ticker      string `json:"ticker"`
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	PublishedTS int64  `json:"published_ts"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("newsvalidate", flag.ContinueOnError)
	configPath := fs.String("config", "config.yaml", "config path")
	classifierPath := fs.String("classifier", "news_classifier.json", "trained classifier JSON")
	newsPath := fs.String("news", "", "news JSONL to score")
	horizon := fs.Int("horizon-days", 3, "forward excess-return horizon in trading days")
	outPath := fs.String("out", "news_scores.csv", "output CSV")
	minTurnoverStr := fs.String("min-turnover", "", "drop articles below this median 20-day turnover in RUB (empty = no filter)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *newsPath == "" {
		return fmt.Errorf("-news is required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	weights, err := model.LoadNewsClassifier(*classifierPath)
	if err != nil {
		return err
	}
	scorer := weights.Scorer()

	f, err := os.Open(*newsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	var articles []model.NewsArticle
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		var r rawArticle
		if json.Unmarshal(scanner.Bytes(), &r) != nil {
			continue
		}
		title := strings.TrimSpace(r.Title)
		if title == "" || r.Ticker == "" || r.PublishedTS <= 0 {
			continue
		}
		articles = append(articles, model.NewsArticle{
			Ticker:      strings.ToUpper(r.Ticker),
			ArticleID:   r.ArticleID,
			Title:       title,
			PublishedAt: time.Unix(r.PublishedTS, 0).UTC(),
		})
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	minTurnover := decimal.Zero
	if *minTurnoverStr != "" {
		parsed, err := decimal.NewFromString(*minTurnoverStr)
		if err != nil {
			return fmt.Errorf("parse -min-turnover: %w", err)
		}
		minTurnover = parsed
	}
	samples, err := model.BuildNewsLabelsWithOptions(context.Background(), source, articles, *horizon, model.NewsLabelOptions{MinTurnover: minTurnover})
	if err != nil {
		return err
	}

	out, err := os.Create(*outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	w := csv.NewWriter(out)
	if err := w.Write([]string{"article_id", "ticker", "date", "score", "label", "excess_return_pct"}); err != nil {
		return err
	}
	for _, s := range samples {
		score, _ := scorer(s.Article.Title).Float64()
		if err := w.Write([]string{
			s.Article.ArticleID,
			s.Article.Ticker,
			s.Article.PublishedAt.Format("2006-01-02"),
			fmt.Sprintf("%.6f", score),
			fmt.Sprintf("%.0f", s.Label),
			fmt.Sprintf("%.4f", s.ExcessReturnPct),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	fmt.Printf("scored %d labeled samples from %d pairs -> %s\n", len(samples), len(articles), *outPath)
	return w.Error()
}
