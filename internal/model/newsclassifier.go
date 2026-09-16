package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
)

// NewsArticle is a single historical headline tied to a ticker, used both to
// build training labels (via forward excess return) and as classifier input.
type NewsArticle struct {
	Ticker      string
	ArticleID   string
	Title       string
	PublishedAt time.Time
}

// NewsTrainingSample is a headline paired with the binary label derived from
// its forward excess return over IMOEX.
type NewsTrainingSample struct {
	Article         NewsArticle
	Label           float64
	ExcessReturnPct float64
}

var newsTokenRe = regexp.MustCompile(`[a-zа-яё0-9]+`)

var newsStopwords = map[string]bool{
	"и": true, "в": true, "во": true, "не": true, "на": true, "с": true, "со": true,
	"по": true, "за": true, "из": true, "к": true, "ко": true, "от": true, "для": true,
	"о": true, "об": true, "что": true, "это": true, "как": true, "но": true, "а": true,
	"же": true, "у": true, "до": true, "его": true, "их": true, "ее": true, "он": true,
	"она": true, "они": true, "то": true, "мы": true, "вы": true, "будет": true,
	"был": true, "была": true, "были": true, "быть": true, "или": true, "также": true,
	"при": true, "после": true, "года": true, "год": true, "млрд": true, "млн": true,
}

func tokenizeTitle(title string) []string {
	low := strings.ToLower(title)
	raw := newsTokenRe.FindAllString(low, -1)
	tokens := make([]string, 0, len(raw))
	for _, tok := range raw {
		if len(tok) < 3 || newsStopwords[tok] {
			continue
		}
		tokens = append(tokens, tok)
	}
	return tokens
}

// NewsVocabConfig bounds the bag-of-words vocabulary built from training titles.
type NewsVocabConfig struct {
	MaxTokens  int
	MinDocFreq int
}

func BuildNewsVocab(titles []string, cfg NewsVocabConfig) []string {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 3000
	}
	if cfg.MinDocFreq <= 0 {
		cfg.MinDocFreq = 3
	}
	docFreq := make(map[string]int)
	for _, title := range titles {
		seen := make(map[string]bool)
		for _, tok := range tokenizeTitle(title) {
			if !seen[tok] {
				docFreq[tok]++
				seen[tok] = true
			}
		}
	}
	type tokenFreq struct {
		token string
		freq  int
	}
	candidates := make([]tokenFreq, 0, len(docFreq))
	for tok, freq := range docFreq {
		if freq >= cfg.MinDocFreq {
			candidates = append(candidates, tokenFreq{tok, freq})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].freq != candidates[j].freq {
			return candidates[i].freq > candidates[j].freq
		}
		return candidates[i].token < candidates[j].token
	})
	if len(candidates) > cfg.MaxTokens {
		candidates = candidates[:cfg.MaxTokens]
	}
	vocab := make([]string, len(candidates))
	for i, c := range candidates {
		vocab[i] = c.token
	}
	sort.Strings(vocab)
	return vocab
}

func buildVocabIndex(vocab []string) map[string]int {
	idx := make(map[string]int, len(vocab))
	for i, tok := range vocab {
		idx[tok] = i
	}
	return idx
}

func vectorizeTitle(title string, vocabIndex map[string]int) []float64 {
	vec := make([]float64, len(vocabIndex))
	for _, tok := range tokenizeTitle(title) {
		if idx, ok := vocabIndex[tok]; ok {
			vec[idx]++
		}
	}
	return vec
}

// BuildNewsLabels looks up, for every article, the trading day it was
// published on and labels it 1 when the ticker's forward excess return over
// IMOEX across horizonDays trading days is positive, 0 otherwise. Entry is
// the next trading day's open (so the label never uses information that
// predates the article) and exit is the close horizonDays trading days later.
func BuildNewsLabels(ctx context.Context, source backtest.HistoricalSource, articles []NewsArticle, horizonDays int) ([]NewsTrainingSample, error) {
	if source == nil {
		return nil, errors.New("model: historical source is required")
	}
	if horizonDays <= 0 {
		horizonDays = 3
	}
	byTicker := make(map[string][]NewsArticle)
	var minTS, maxTS time.Time
	for _, a := range articles {
		if a.Ticker == "" || a.PublishedAt.IsZero() {
			continue
		}
		byTicker[a.Ticker] = append(byTicker[a.Ticker], a)
		if minTS.IsZero() || a.PublishedAt.Before(minTS) {
			minTS = a.PublishedAt
		}
		if a.PublishedAt.After(maxTS) {
			maxTS = a.PublishedAt
		}
	}
	if minTS.IsZero() {
		return nil, nil
	}
	fetchFrom := minTS.AddDate(0, 0, -5)
	fetchTill := maxTS.AddDate(0, 0, horizonDays+10)

	indexCandles, err := source.History(ctx, benchmarkTicker, fetchFrom, fetchTill)
	if err != nil {
		return nil, fmt.Errorf("model: history %s: %w", benchmarkTicker, err)
	}
	indexByDate := indexCandlesByDate(indexCandles)

	var samples []NewsTrainingSample
	for ticker, list := range byTicker {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if ticker == benchmarkTicker {
			continue
		}
		candles, err := source.History(ctx, ticker, fetchFrom, fetchTill)
		if err != nil || len(candles) < horizonDays+2 {
			continue
		}
		for _, article := range list {
			pubIdx := -1
			for i, c := range candles {
				if !c.Begin.Before(article.PublishedAt) {
					pubIdx = i
					break
				}
			}
			if pubIdx < 0 {
				continue
			}
			entryIdx := pubIdx + 1
			exitIdx := pubIdx + 1 + horizonDays
			if exitIdx >= len(candles) {
				continue
			}
			entryCandle := candles[entryIdx]
			exitCandle := candles[exitIdx]
			entry := entryCandle.Open
			exit := exitCandle.Close
			if entry.Sign() <= 0 || exit.Sign() <= 0 {
				continue
			}
			indexEntry, ok := indexByDate[dateKey(entryCandle.Begin)]
			if !ok || indexEntry.Open.Sign() <= 0 {
				continue
			}
			indexExit, ok := indexByDate[dateKey(exitCandle.Begin)]
			if !ok || indexExit.Close.Sign() <= 0 {
				continue
			}
			forwardReturn := exit.Sub(entry).Div(entry).Mul(decimal.NewFromInt(100))
			indexForwardReturn := indexExit.Close.Sub(indexEntry.Open).Div(indexEntry.Open).Mul(decimal.NewFromInt(100))
			excessPct, _ := forwardReturn.Sub(indexForwardReturn).Float64()

			label := 0.0
			if excessPct > 0 {
				label = 1.0
			}
			samples = append(samples, NewsTrainingSample{Article: article, Label: label, ExcessReturnPct: excessPct})
		}
	}
	return samples, nil
}

// NewsClassifierMetadata records how a trained news classifier performed.
type NewsClassifierMetadata struct {
	TrainSamples     int     `json:"train_samples"`
	ValSamples       int     `json:"val_samples"`
	TrainAUC         float64 `json:"train_auc"`
	ValAUC           float64 `json:"val_auc"`
	ValAccuracy      float64 `json:"val_accuracy"`
	BaselineAccuracy float64 `json:"baseline_accuracy"`
}

// NewsClassifierWeights is the persisted artifact for a bag-of-words logistic
// regression trained to predict a headline's forward excess-return direction.
type NewsClassifierWeights struct {
	Vocab       []string               `json:"vocab"`
	Mean        []float64              `json:"mean"`
	Std         []float64              `json:"std"`
	Coef        []float64              `json:"coef"`
	Bias        float64                `json:"bias"`
	HorizonDays int                    `json:"horizon_days"`
	TrainedAt   time.Time              `json:"trained_at"`
	Training    NewsClassifierMetadata `json:"training"`
}

func (w *NewsClassifierWeights) validate() error {
	if len(w.Coef) != len(w.Vocab) || len(w.Coef) != len(w.Mean) || len(w.Coef) != len(w.Std) {
		return fmt.Errorf("dimension mismatch: coef=%d vocab=%d mean=%d std=%d",
			len(w.Coef), len(w.Vocab), len(w.Mean), len(w.Std))
	}
	return nil
}

func (w *NewsClassifierWeights) Save(path string) error {
	if err := w.validate(); err != nil {
		return fmt.Errorf("model: validate news classifier before save: %w", err)
	}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Errorf("model: marshal news classifier: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("model: create dir for %q: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("model: write news classifier %q: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("model: replace news classifier %q: %w", path, err)
	}
	return nil
}

func LoadNewsClassifier(path string) (*NewsClassifierWeights, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("model: read news classifier %q: %w", path, err)
	}
	var w NewsClassifierWeights
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("model: parse news classifier %q: %w", path, err)
	}
	if err := w.validate(); err != nil {
		return nil, fmt.Errorf("model: invalid news classifier %q: %w", path, err)
	}
	return &w, nil
}

// Scorer returns a PolarityScore/RegexScore-compatible function: it maps a
// headline to a signed score in [-1, 1] via 2*p-1, p being the trained
// probability that the headline's forward excess return is positive.
func (w *NewsClassifierWeights) Scorer() func(string) decimal.Decimal {
	vocabIndex := buildVocabIndex(w.Vocab)
	mean, std, coef, bias := w.Mean, w.Std, w.Coef, w.Bias
	return func(title string) decimal.Decimal {
		x := vectorizeTitle(title, vocabIndex)
		z := bias
		for i, v := range x {
			standardized := (v - mean[i]) / std[i]
			z += standardized * coef[i]
		}
		p := sigmoid(z)
		return decimal.NewFromFloat(2*p - 1)
	}
}

// TrainNewsClassifierConfig controls label construction, vocabulary size, and
// the underlying logistic regression optimizer for TrainNewsClassifier.
type TrainNewsClassifierConfig struct {
	HorizonDays int
	VocabSize   int
	MinDocFreq  int
	Split       time.Time
	TrainCfg    TrainConfig
}

// TrainNewsClassifier builds forward-excess-return labels for articles, fits
// a bag-of-words logistic regression on the split before cfg.Split, and
// reports train/validation AUC and accuracy (against a majority-class
// baseline) on the split after it.
func TrainNewsClassifier(ctx context.Context, source backtest.HistoricalSource, articles []NewsArticle, cfg TrainNewsClassifierConfig) (*NewsClassifierWeights, error) {
	samples, err := BuildNewsLabels(ctx, source, articles, cfg.HorizonDays)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, errors.New("model: no labeled news samples in range")
	}

	var trainSamples, valSamples []NewsTrainingSample
	for _, s := range samples {
		if s.Article.PublishedAt.Before(cfg.Split) {
			trainSamples = append(trainSamples, s)
		} else {
			valSamples = append(valSamples, s)
		}
	}
	if len(trainSamples) == 0 {
		return nil, errors.New("model: no training samples before the split date")
	}

	vocab := BuildNewsVocab(newsTitles(trainSamples), NewsVocabConfig{MaxTokens: cfg.VocabSize, MinDocFreq: cfg.MinDocFreq})
	if len(vocab) == 0 {
		return nil, errors.New("model: empty vocabulary, lower -min-doc-freq or add more training data")
	}
	vocabIndex := buildVocabIndex(vocab)

	rows := make([]Sample, len(trainSamples))
	for i, s := range trainSamples {
		rows[i] = Sample{X: vectorizeTitle(s.Article.Title, vocabIndex), Y: s.Label}
	}
	mean, std := Standardize(rows)
	coef, bias, err := Train(rows, cfg.TrainCfg)
	if err != nil {
		return nil, fmt.Errorf("model: train news classifier: %w", err)
	}

	w := &NewsClassifierWeights{
		Vocab:       vocab,
		Mean:        mean,
		Std:         std,
		Coef:        coef,
		Bias:        bias,
		HorizonDays: cfg.HorizonDays,
		TrainedAt:   time.Now(),
	}
	scorer := w.Scorer()
	valAcc, baselineAcc := newsAccuracy(valSamples, scorer)
	w.Training = NewsClassifierMetadata{
		TrainSamples:     len(trainSamples),
		ValSamples:       len(valSamples),
		TrainAUC:         newsAUC(trainSamples, scorer),
		ValAUC:           newsAUC(valSamples, scorer),
		ValAccuracy:      valAcc,
		BaselineAccuracy: baselineAcc,
	}
	return w, nil
}

func newsTitles(samples []NewsTrainingSample) []string {
	out := make([]string, len(samples))
	for i, s := range samples {
		out[i] = s.Article.Title
	}
	return out
}

// newsAUC computes the rank-based (Mann-Whitney) AUC of scorer against the
// samples' binary labels; ties share the average rank. Returns 0.5 when a
// class is absent (AUC undefined).
func newsAUC(samples []NewsTrainingSample, scorer func(string) decimal.Decimal) float64 {
	type scoredSample struct {
		score float64
		label float64
	}
	scored := make([]scoredSample, 0, len(samples))
	pos, neg := 0, 0
	for _, s := range samples {
		v, _ := scorer(s.Article.Title).Float64()
		scored = append(scored, scoredSample{score: v, label: s.Label})
		if s.Label == 1 {
			pos++
		} else {
			neg++
		}
	}
	if pos == 0 || neg == 0 {
		return 0.5
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].score < scored[j].score })
	var rankSum float64
	i := 0
	for i < len(scored) {
		j := i
		for j < len(scored) && scored[j].score == scored[i].score {
			j++
		}
		avgRank := float64(i+j+1) / 2.0
		for k := i; k < j; k++ {
			if scored[k].label == 1 {
				rankSum += avgRank
			}
		}
		i = j
	}
	return (rankSum - float64(pos)*float64(pos+1)/2) / (float64(pos) * float64(neg))
}

func newsAccuracy(samples []NewsTrainingSample, scorer func(string) decimal.Decimal) (accuracy, baseline float64) {
	if len(samples) == 0 {
		return 0, 0
	}
	correct, pos := 0, 0
	for _, s := range samples {
		v, _ := scorer(s.Article.Title).Float64()
		pred := 0.0
		if v > 0 {
			pred = 1.0
		}
		if pred == s.Label {
			correct++
		}
		if s.Label == 1 {
			pos++
		}
	}
	majority := pos
	if neg := len(samples) - pos; neg > majority {
		majority = neg
	}
	return float64(correct) / float64(len(samples)), float64(majority) / float64(len(samples))
}
