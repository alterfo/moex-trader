package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

const defaultConfigPath = "config.yaml"

type options struct {
	configPath  string
	newsPath    string
	splitStr    string
	horizonDays int
	vocabSize   int
	minDocFreq  int
	outPath     string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	split, err := time.Parse("2006-01-02", opts.splitStr)
	if err != nil {
		return fmt.Errorf("parse -split: %w", err)
	}

	articles, err := loadNewsArticles(opts.newsPath)
	if err != nil {
		return fmt.Errorf("load news: %w", err)
	}
	if len(articles) == 0 {
		return errors.New("no usable articles found (missing title/ticker/published_ts?)")
	}
	log.Printf("trainnewsmodel: loaded %d ticker-headline pairs from %s", len(articles), opts.newsPath)

	moexClient := moex.NewClient(cfg.MOEXISSBaseURL, nil)
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moexClient)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	weights, err := model.TrainNewsClassifier(ctx, source, articles, model.TrainNewsClassifierConfig{
		HorizonDays: opts.horizonDays,
		VocabSize:   opts.vocabSize,
		MinDocFreq:  opts.minDocFreq,
		Split:       split,
		TrainCfg:    model.DefaultTrainConfig(),
	})
	if err != nil {
		return fmt.Errorf("train news classifier: %w", err)
	}

	if err := weights.Save(opts.outPath); err != nil {
		return fmt.Errorf("save %q: %w", opts.outPath, err)
	}

	t := weights.Training
	fmt.Fprintf(stdout, "vocab=%d train_samples=%d val_samples=%d train_auc=%.3f val_auc=%.3f val_accuracy=%.3f baseline_accuracy=%.3f -> %s\n",
		len(weights.Vocab), t.TrainSamples, t.ValSamples, t.TrainAUC, t.ValAUC, t.ValAccuracy, t.BaselineAccuracy, opts.outPath)
	return nil
}

type rawArticle struct {
	Ticker      string `json:"ticker"`
	ArticleID   string `json:"article_id"`
	Title       string `json:"title"`
	PublishedTS int64  `json:"published_ts"`
}

func loadNewsArticles(path string) ([]model.NewsArticle, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var articles []model.NewsArticle
	for scanner.Scan() {
		var r rawArticle
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		title := strings.TrimSpace(r.Title)
		ticker := strings.ToUpper(strings.TrimSpace(r.Ticker))
		if title == "" || strings.EqualFold(title, "no title") || ticker == "" || r.PublishedTS <= 0 {
			continue
		}
		articles = append(articles, model.NewsArticle{
			Ticker:      ticker,
			ArticleID:   r.ArticleID,
			Title:       title,
			PublishedAt: time.Unix(r.PublishedTS, 0).UTC(),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return articles, nil
}

func parseOptions(args []string) (options, error) {
	opts := options{
		configPath:  defaultConfigPath,
		horizonDays: 3,
		vocabSize:   3000,
		minDocFreq:  3,
		outPath:     "news_classifier.json",
	}
	fs := flag.NewFlagSet("trainnewsmodel", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", opts.configPath, "config path")
	fs.StringVar(&opts.newsPath, "news", "", "raw news JSONL: {ticker,article_id,title,published_ts} per line (required)")
	fs.StringVar(&opts.splitStr, "split", "", "train/validation split YYYY-MM-DD: articles before it train, after it validate (required)")
	fs.IntVar(&opts.horizonDays, "horizon-days", opts.horizonDays, "forward excess-return horizon in trading days")
	fs.IntVar(&opts.vocabSize, "vocab-size", opts.vocabSize, "max bag-of-words vocabulary size")
	fs.IntVar(&opts.minDocFreq, "min-doc-freq", opts.minDocFreq, "minimum document frequency for a token to enter the vocabulary")
	fs.StringVar(&opts.outPath, "out", opts.outPath, "output path for the trained classifier JSON")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() > 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if opts.newsPath == "" {
		return options{}, errors.New("-news is required")
	}
	if opts.splitStr == "" {
		return options{}, errors.New("-split is required")
	}
	return opts, nil
}
