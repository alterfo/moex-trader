package model

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

func TestTokenizeTitleFiltersStopwordsAndShortTokens(t *testing.T) {
	got := tokenizeTitle("Газпром и не Роснефть, а Сбербанк на 10% вырос")
	want := []string{"газпром", "роснефть", "сбербанк", "вырос"}
	if len(got) != len(want) {
		t.Fatalf("tokenizeTitle() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tokenizeTitle()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestBuildNewsVocabRespectsMinDocFreqAndMaxTokens(t *testing.T) {
	titles := []string{
		"рост прибыли газпром",
		"рост прибыли лукойл",
		"рост выручки сбербанк",
		"убыток санкции авария",
	}
	vocab := BuildNewsVocab(titles, NewsVocabConfig{MaxTokens: 100, MinDocFreq: 2})
	idx := buildVocabIndex(vocab)
	if _, ok := idx["рост"]; !ok {
		t.Errorf("vocab missing %q (doc freq 3), got %v", "рост", vocab)
	}
	if _, ok := idx["прибыли"]; !ok {
		t.Errorf("vocab missing %q (doc freq 2), got %v", "прибыли", vocab)
	}
	if _, ok := idx["убыток"]; ok {
		t.Errorf("vocab contains %q with doc freq 1, want excluded, got %v", "убыток", vocab)
	}

	capped := BuildNewsVocab(titles, NewsVocabConfig{MaxTokens: 1, MinDocFreq: 1})
	if len(capped) != 1 {
		t.Fatalf("BuildNewsVocab() with MaxTokens=1 returned %d tokens, want 1", len(capped))
	}
}

func dailyCandle(day time.Time, open, close float64) moex.Candle {
	return moex.Candle{
		Open:  decimal.NewFromFloat(open),
		Close: decimal.NewFromFloat(close),
		High:  decimal.NewFromFloat(open + 1),
		Low:   decimal.NewFromFloat(open - 1),
		Begin: day,
		End:   day.Add(18 * time.Hour),
	}
}

// buildNewsTestSource returns a source where IMOEX is flat every day and the
// given ticker's daily open/close series is exactly returns (as a fraction,
// e.g. 0.01 for +1%) applied compounding from a base price of 100, starting
// on baseDay for n trading days.
func buildNewsTestSource(ticker string, baseDay time.Time, returns []float64) datasetSource {
	n := len(returns) + 1
	tickerCandles := make([]moex.Candle, n)
	indexCandles := make([]moex.Candle, n)
	price := 100.0
	for i := 0; i < n; i++ {
		day := baseDay.AddDate(0, 0, i)
		open := price
		close := price
		if i > 0 {
			close = price * (1 + returns[i-1])
		}
		tickerCandles[i] = dailyCandle(day, open, close)
		indexCandles[i] = dailyCandle(day, 3000, 3000)
		price = close
	}
	return datasetSource{series: map[string][]moex.Candle{
		ticker:          tickerCandles,
		benchmarkTicker: indexCandles,
	}}
}

func TestBuildNewsLabelsPositiveExcessReturn(t *testing.T) {
	baseDay := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Article published on day 0; entry = day1 open, exit = day1+horizon close.
	// Day1 return +5%, day2 flat, day3 flat -> 3-day horizon forward return > 0, IMOEX flat -> positive excess.
	source := buildNewsTestSource("SBER", baseDay, []float64{0.05, 0, 0, 0})
	articles := []NewsArticle{{Ticker: "SBER", ArticleID: "a1", Title: "рост прибыли", PublishedAt: baseDay}}

	samples, err := BuildNewsLabels(context.Background(), source, articles, 3)
	if err != nil {
		t.Fatalf("BuildNewsLabels() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("BuildNewsLabels() returned %d samples, want 1", len(samples))
	}
	if samples[0].Label != 1 {
		t.Errorf("Label = %v, want 1 (excess=%.4f)", samples[0].Label, samples[0].ExcessReturnPct)
	}
	if samples[0].ExcessReturnPct <= 0 {
		t.Errorf("ExcessReturnPct = %v, want > 0", samples[0].ExcessReturnPct)
	}
}

func TestBuildNewsLabelsNegativeExcessReturn(t *testing.T) {
	baseDay := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	source := buildNewsTestSource("GAZP", baseDay, []float64{-0.05, 0, 0, 0})
	articles := []NewsArticle{{Ticker: "GAZP", ArticleID: "a1", Title: "убыток санкции", PublishedAt: baseDay}}

	samples, err := BuildNewsLabels(context.Background(), source, articles, 3)
	if err != nil {
		t.Fatalf("BuildNewsLabels() error = %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("BuildNewsLabels() returned %d samples, want 1", len(samples))
	}
	if samples[0].Label != 0 {
		t.Errorf("Label = %v, want 0 (excess=%.4f)", samples[0].Label, samples[0].ExcessReturnPct)
	}
}

func TestBuildNewsLabelsSkipsArticlesWithoutEnoughFutureHistory(t *testing.T) {
	baseDay := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	source := buildNewsTestSource("SBER", baseDay, []float64{0.05})
	// Only 2 candles total; horizon=3 needs exitIdx=pubIdx+1+3=4, out of range.
	articles := []NewsArticle{{Ticker: "SBER", ArticleID: "a1", Title: "рост", PublishedAt: baseDay}}

	samples, err := BuildNewsLabels(context.Background(), source, articles, 3)
	if err != nil {
		t.Fatalf("BuildNewsLabels() error = %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("BuildNewsLabels() returned %d samples, want 0 (insufficient history)", len(samples))
	}
}

// TestTrainNewsClassifierLearnsSeparableSignal builds a synthetic corpus
// where headlines containing "рост" always precede a positive excess return
// and headlines containing "падение" always precede a negative one, then
// checks the trained classifier separates them out of sample.
func TestTrainNewsClassifierLearnsSeparableSignal(t *testing.T) {
	baseDay := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	const n = 200
	tickerCandles := make([]moex.Candle, 0, n*4)
	indexCandles := make([]moex.Candle, 0, n*4)
	var articles []NewsArticle
	day := baseDay
	price := 100.0
	for i := 0; i < n; i++ {
		up := i%2 == 0
		ret := -0.04
		title := "падение выручки убыток"
		if up {
			ret = 0.04
			title = "рост прибыли рекорд"
		}
		pubDay := day
		// Candle A (pubIdx): flat at the pre-move price, published here.
		tickerCandles = append(tickerCandles, dailyCandle(day, price, price))
		indexCandles = append(indexCandles, dailyCandle(day, 3000, 3000))
		day = day.AddDate(0, 0, 1)
		// Candle B (entryIdx = pubIdx+1): the move happens here; entry uses its Open.
		moved := price * (1 + ret)
		tickerCandles = append(tickerCandles, dailyCandle(day, price, moved))
		indexCandles = append(indexCandles, dailyCandle(day, 3000, 3000))
		day = day.AddDate(0, 0, 1)
		// Candle C: holds the moved level.
		tickerCandles = append(tickerCandles, dailyCandle(day, moved, moved))
		indexCandles = append(indexCandles, dailyCandle(day, 3000, 3000))
		day = day.AddDate(0, 0, 1)
		// Candle D (exitIdx = pubIdx+1+horizon(2)): still holds the moved level; exit uses its Close.
		tickerCandles = append(tickerCandles, dailyCandle(day, moved, moved))
		indexCandles = append(indexCandles, dailyCandle(day, 3000, 3000))
		day = day.AddDate(0, 0, 1)
		price = moved

		articles = append(articles, NewsArticle{
			Ticker: "SBER", ArticleID: "a" + strconv.Itoa(i), Title: title, PublishedAt: pubDay,
		})
	}
	source := datasetSource{series: map[string][]moex.Candle{
		"SBER":          tickerCandles,
		benchmarkTicker: indexCandles,
	}}

	split := baseDay.AddDate(0, 0, 4*n*3/4)
	w, err := TrainNewsClassifier(context.Background(), source, articles, TrainNewsClassifierConfig{
		HorizonDays: 2,
		VocabSize:   50,
		MinDocFreq:  2,
		Split:       split,
		TrainCfg:    TrainConfig{LearningRate: 0.5, L2Lambda: 0.001, Epochs: 300},
	})
	if err != nil {
		t.Fatalf("TrainNewsClassifier() error = %v", err)
	}
	if w.Training.ValSamples == 0 {
		t.Fatal("expected non-zero validation samples")
	}
	if w.Training.ValAUC < 0.9 {
		t.Errorf("ValAUC = %.3f, want >= 0.9 on a trivially separable signal", w.Training.ValAUC)
	}

	scorer := w.Scorer()
	upScore, _ := scorer("рост прибыли рекорд").Float64()
	downScore, _ := scorer("падение выручки убыток").Float64()
	if upScore <= downScore {
		t.Errorf("scorer(up)=%.3f should exceed scorer(down)=%.3f", upScore, downScore)
	}
	if upScore <= 0 {
		t.Errorf("scorer(up) = %.3f, want > 0", upScore)
	}
	if downScore >= 0 {
		t.Errorf("scorer(down) = %.3f, want < 0", downScore)
	}
}

func TestNewsClassifierSaveLoadRoundTrip(t *testing.T) {
	w := &NewsClassifierWeights{
		Vocab:       []string{"рост", "падение"},
		Mean:        []float64{0.1, 0.2},
		Std:         []float64{1, 1},
		Coef:        []float64{0.5, -0.5},
		Bias:        0.01,
		HorizonDays: 3,
		TrainedAt:   time.Now().UTC().Truncate(time.Second),
	}
	path := filepath.Join(t.TempDir(), "news_classifier.json")
	if err := w.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := LoadNewsClassifier(path)
	if err != nil {
		t.Fatalf("LoadNewsClassifier() error = %v", err)
	}
	if len(loaded.Vocab) != len(w.Vocab) || loaded.Bias != w.Bias {
		t.Fatalf("LoadNewsClassifier() = %+v, want match of %+v", loaded, w)
	}
}

func TestNewsClassifierSaveRejectsDimensionMismatch(t *testing.T) {
	w := &NewsClassifierWeights{Vocab: []string{"a", "b"}, Mean: []float64{0}, Std: []float64{1}, Coef: []float64{0.1}}
	if err := w.Save(filepath.Join(t.TempDir(), "bad.json")); err == nil {
		t.Fatal("Save() error = nil, want dimension mismatch error")
	}
}

func TestLoadNewsClassifierMissingFile(t *testing.T) {
	if _, err := LoadNewsClassifier(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("LoadNewsClassifier() error = nil, want error for missing file")
	}
}
