package model

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/config"
	"github.com/olegsidorkin/moex-trader/internal/ingestion/moex"
)

type dropAudit struct {
	articlesIn        int
	droppedNoTicker   int
	skippedBenchmark  int
	pairsIn           int
	tickersIn         int
	dropATickers      []string
	dropAHistoryErr   []string
	dropAPairsErr     int
	dropAHistoryShort []string
	dropAPairsShort   int
	dropBPubIdx       int
	dropCExitIdx      int
	dropDPrice        int
	dropEIndexEntry   int
	dropFIndexExit    int
	samples           int
	missingIndexDates map[string]int
	indexCandleCount  int
	indexFirst        string
	indexLast         string
}

type auditRaw struct {
	Title       string `json:"title"`
	Ticker      string `json:"ticker"`
	ArticleID   string `json:"article_id"`
	PublishedTS int64  `json:"published_ts"`
}

func loadAuditArticles(t *testing.T, path string) []NewsArticle {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("archive %s unavailable: %v", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	var articles []NewsArticle
	for scanner.Scan() {
		var r auditRaw
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			continue
		}
		title := strings.TrimSpace(r.Title)
		ticker := strings.ToUpper(strings.TrimSpace(r.Ticker))
		if title == "" || strings.EqualFold(title, "no title") || ticker == "" || r.PublishedTS <= 0 {
			continue
		}
		articles = append(articles, NewsArticle{
			Ticker:      ticker,
			ArticleID:   r.ArticleID,
			Title:       title,
			PublishedAt: time.Unix(r.PublishedTS, 0).UTC(),
		})
	}
	return articles
}

func auditBuildNewsLabels(ctx context.Context, source backtest.HistoricalSource, articles []NewsArticle, horizonDays int) (dropAudit, []NewsTrainingSample, error) {
	var a dropAudit
	a.missingIndexDates = map[string]int{}
	a.articlesIn = len(articles)

	byTicker := make(map[string][]NewsArticle)
	var minTS, maxTS time.Time
	for _, art := range articles {
		if art.Ticker == "" || art.PublishedAt.IsZero() {
			a.droppedNoTicker++
			continue
		}
		byTicker[art.Ticker] = append(byTicker[art.Ticker], art)
		if minTS.IsZero() || art.PublishedAt.Before(minTS) {
			minTS = art.PublishedAt
		}
		if art.PublishedAt.After(maxTS) {
			maxTS = art.PublishedAt
		}
	}
	fetchFrom := minTS.AddDate(0, 0, -5)
	fetchTill := maxTS.AddDate(0, 0, horizonDays+10)

	indexCandles, err := source.History(ctx, benchmarkTicker, fetchFrom, fetchTill)
	if err != nil {
		return a, nil, err
	}
	indexByDate := indexCandlesByDate(indexCandles)
	a.indexCandleCount = len(indexCandles)
	if len(indexCandles) > 0 {
		a.indexFirst = dateKey(indexCandles[0].Begin)
		a.indexLast = dateKey(indexCandles[len(indexCandles)-1].Begin)
	}

	var samples []NewsTrainingSample
	for ticker, list := range byTicker {
		if ticker == benchmarkTicker {
			a.skippedBenchmark += len(list)
			continue
		}
		a.tickersIn++
		a.pairsIn += len(list)
		candles, err := source.History(ctx, ticker, fetchFrom, fetchTill)
		if err != nil {
			a.dropATickers = append(a.dropATickers, ticker)
			a.dropAPairsErr += len(list)
			a.dropAHistoryErr = append(a.dropAHistoryErr, fmt.Sprintf("%s: %d articles: %v", ticker, len(list), err))
			continue
		}
		if len(candles) < horizonDays+2 {
			a.dropATickers = append(a.dropATickers, ticker)
			a.dropAPairsShort += len(list)
			a.dropAHistoryShort = append(a.dropAHistoryShort,
				fmt.Sprintf("%s: %d articles, %d candles < %d required", ticker, len(list), len(candles), horizonDays+2))
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
				a.dropBPubIdx++
				continue
			}
			entryIdx := pubIdx + 1
			exitIdx := pubIdx + 1 + horizonDays
			if exitIdx >= len(candles) {
				a.dropCExitIdx++
				continue
			}
			entryCandle := candles[entryIdx]
			exitCandle := candles[exitIdx]
			entry := entryCandle.Open
			exit := exitCandle.Close
			if entry.Sign() <= 0 || exit.Sign() <= 0 {
				a.dropDPrice++
				continue
			}
			indexEntry, ok := indexByDate[dateKey(entryCandle.Begin)]
			if !ok || indexEntry.Open.Sign() <= 0 {
				a.dropEIndexEntry++
				a.missingIndexDates[dateKey(entryCandle.Begin)]++
				continue
			}
			indexExit, ok := indexByDate[dateKey(exitCandle.Begin)]
			if !ok || indexExit.Close.Sign() <= 0 {
				a.dropFIndexExit++
				a.missingIndexDates[dateKey(exitCandle.Begin)]++
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
	a.samples = len(samples)
	return a, samples, nil
}

func TestNewsLabelsDropAudit(t *testing.T) {
	archive := os.Getenv("MOEX_TRADER_NEWS_AUDIT_ARCHIVE")
	if archive == "" {
		t.Skip("set MOEX_TRADER_NEWS_AUDIT_ARCHIVE to audit news-label drop points")
	}
	cfg, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	source := backtest.NewISSSource(cfg.MOEXISSBaseURL, moex.NewClient(cfg.MOEXISSBaseURL, nil))
	articles := loadAuditArticles(t, archive)

	ctx := context.Background()
	a, samples, err := auditBuildNewsLabels(ctx, source, articles, 3)
	if err != nil {
		t.Fatalf("audit run: %v", err)
	}
	real, err := BuildNewsLabels(ctx, source, articles, 3)
	if err != nil {
		t.Fatalf("real BuildNewsLabels: %v", err)
	}
	if len(samples) != len(real) {
		t.Fatalf("audit replica diverges from BuildNewsLabels: %d vs %d", len(samples), len(real))
	}

	t.Logf("INPUT articles                 : %d", a.articlesIn)
	t.Logf("  dropped empty ticker/date    : %d", a.droppedNoTicker)
	t.Logf("PAIRS entering ticker loop     : %d (tickers=%d, benchmark skipped=%d)", a.pairsIn, a.tickersIn, a.skippedBenchmark)
	t.Logf("(a) ticker dropped, history error : %d articles across %d tickers", a.dropAPairsErr, len(a.dropAHistoryErr))
	for _, s := range a.dropAHistoryErr {
		t.Logf("      err   %s", s)
	}
	t.Logf("(a) ticker dropped, too short    : %d articles across %d tickers", a.dropAPairsShort, len(a.dropAHistoryShort))
	for _, s := range a.dropAHistoryShort {
		t.Logf("      short %s", s)
	}
	t.Logf("(b) article pubIdx<0           : %d", a.dropBPubIdx)
	t.Logf("(c) article exitIdx overflow   : %d", a.dropCExitIdx)
	t.Logf("(d) entry/exit price <= 0     : %d", a.dropDPrice)
	t.Logf("(e) IMOEX entry candle missing: %d", a.dropEIndexEntry)
	t.Logf("(f) IMOEX exit candle missing : %d", a.dropFIndexExit)
	t.Logf("SAMPLES                        : %d", a.samples)
	split := os.Getenv("MOEX_TRADER_NEWS_AUDIT_SPLIT")
	if split == "" {
		split = "2026-08-15"
	}
	t.Logf("IMOEX candles fetched          : %d (%s .. %s)", a.indexCandleCount, a.indexFirst, a.indexLast)
	t.Logf("dates missing from IMOEX index  : %d distinct", len(a.missingIndexDates))
	byWeekday := map[time.Weekday]int{}
	articlesOnWeekend := 0
	for d, n := range a.missingIndexDates {
		parsed, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue
		}
		byWeekday[parsed.Weekday()] += n
		if parsed.Weekday() == time.Saturday || parsed.Weekday() == time.Sunday {
			articlesOnWeekend += n
		}
	}
	names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		if byWeekday[wd] > 0 {
			t.Logf("      %s: %d article-dates missing IMOEX", names[wd], byWeekday[wd])
		}
	}
	t.Logf("      -> weekend-caused: %d of %d article-dates", articlesOnWeekend, a.dropEIndexEntry+a.dropFIndexExit)
	type kv struct {
		d string
		n int
	}
	var top []kv
	for d, n := range a.missingIndexDates {
		top = append(top, kv{d, n})
	}
	sort.Slice(top, func(i, j int) bool { return top[i].n > top[j].n })
	for i, e := range top {
		if i >= 8 {
			break
		}
		t.Logf("      %s missing for %d articles", e.d, e.n)
	}
	splitDate, _ := time.Parse("2006-01-02", split)
	tr, vl := 0, 0
	for _, s := range samples {
		if s.Article.PublishedAt.Before(splitDate) {
			tr++
		} else {
			vl++
		}
	}
	t.Logf("split at %s                    : train=%d val=%d total=%d", split, tr, vl, len(samples))
}

func dropDroppedPairs(a dropAudit) int {
	return len(a.dropAHistoryErr)
}

func droppedPairsShort(a dropAudit) int {
	return len(a.dropAHistoryShort)
}
