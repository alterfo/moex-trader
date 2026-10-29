package memberagree

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/olegsidorkin/moex-trader/internal/backtest"
	"github.com/olegsidorkin/moex-trader/internal/domain"
	"github.com/olegsidorkin/moex-trader/internal/model"
)

// Decision is the member-probability triple seen at one decision bar.
type Decision struct {
	Ticker             string
	Date               time.Time
	LGB, XGB, Logistic float64
}

// Recorder wraps a signal source and stores the member probabilities of every
// decision it produces, so the split of closed trades by member agreement does
// not need a second pass over the features.
type Recorder struct {
	inner    backtest.SignalSource
	ensemble *model.EnsembleModel

	mu        sync.Mutex
	decisions map[string]Decision
}

func NewRecorder(inner backtest.SignalSource, ensemble *model.EnsembleModel) *Recorder {
	return &Recorder{inner: inner, ensemble: ensemble, decisions: make(map[string]Decision)}
}

func (r *Recorder) Generate(ctx context.Context, feature domain.FeatureContext) (domain.TradeSignal, error) {
	signal, err := r.inner.Generate(ctx, feature)
	if err != nil {
		return signal, err
	}
	lgb, xgb, logistic, memberErr := r.ensemble.MemberProbabilitiesFor(feature)
	if memberErr != nil {
		return signal, nil
	}
	key := decisionKey(feature.Ticker, feature.GeneratedAt)
	r.mu.Lock()
	r.decisions[key] = Decision{Ticker: strings.ToUpper(feature.Ticker), Date: feature.GeneratedAt, LGB: lgb, XGB: xgb, Logistic: logistic}
	r.mu.Unlock()
	return signal, nil
}

func (r *Recorder) VolScaleEnabled() bool {
	aware, ok := r.inner.(backtest.VolScaleAware)
	return ok && aware.VolScaleEnabled()
}

func (r *Recorder) TargetPosition(feature domain.FeatureContext) (int, bool) {
	target, ok := r.inner.(backtest.TargetPositionSource)
	if !ok {
		return 0, false
	}
	return target.TargetPosition(feature)
}

func (r *Recorder) Decisions() []Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Decision, 0, len(r.decisions))
	for _, decision := range r.decisions {
		out = append(out, decision)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].Ticker < out[j].Ticker
		}
		return out[i].Date.Before(out[j].Date)
	})
	return out
}

func decisionKey(ticker string, day time.Time) string {
	return strings.ToUpper(ticker) + "|" + day.Format("2006-01-02")
}

// Row is one closed trade with the member probabilities of the decision that
// opened it.
type Row struct {
	Window             string
	Seed               int
	Ticker             string
	Action             string
	OpenedAt           time.Time
	ClosedAt           time.Time
	Lots               int
	NetPnl             decimal.Decimal
	Ensemble           float64
	LGB, XGB, Logistic float64
	Unanimous          bool
	SignAgreement      bool
}

func (r Row) Spread() float64 {
	return math.Max(r.LGB, math.Max(r.XGB, r.Logistic)) - math.Min(r.LGB, math.Min(r.XGB, r.Logistic))
}

func (r Row) Conviction() float64 {
	return math.Abs(r.Ensemble - 0.5)
}

// Join attaches the decision-time member probabilities to the closed trades of
// one replay. Trades opened without a model decision (target-flat, time-exit,
// risk-gate rejections that later close a position) are counted as unmatched.
func Join(trades []backtest.Trade, decisions []Decision, buyThreshold, sellThreshold float64) (rows []Row, unmatched int) {
	byKey := make(map[string]Decision, len(decisions))
	for _, decision := range decisions {
		byKey[decisionKey(decision.Ticker, decision.Date)] = decision
	}
	for _, trade := range trades {
		decision, ok := byKey[decisionKey(trade.Ticker, trade.OpenedAt)]
		if !ok {
			unmatched++
			continue
		}
		ensemble := (decision.LGB + decision.XGB + decision.Logistic) / 3.0
		row := Row{
			Ticker:        strings.ToUpper(trade.Ticker),
			Action:        string(trade.Action),
			OpenedAt:      trade.OpenedAt,
			ClosedAt:      trade.ClosedAt,
			Lots:          trade.Lots,
			NetPnl:        trade.NetPnl,
			Ensemble:      ensemble,
			LGB:           decision.LGB,
			XGB:           decision.XGB,
			Logistic:      decision.Logistic,
			Unanimous:     unanimous(trade.Action, decision, buyThreshold, sellThreshold),
			SignAgreement: signAgreement(decision),
		}
		rows = append(rows, row)
	}
	return rows, unmatched
}

func unanimous(action domain.Action, decision Decision, buyThreshold, sellThreshold float64) bool {
	switch action {
	case domain.ActionBuy:
		return decision.LGB >= buyThreshold && decision.XGB >= buyThreshold && decision.Logistic >= buyThreshold
	case domain.ActionSell:
		return decision.LGB <= sellThreshold && decision.XGB <= sellThreshold && decision.Logistic <= sellThreshold
	default:
		return false
	}
}

func signAgreement(decision Decision) bool {
	up := decision.LGB > 0.5 && decision.XGB > 0.5 && decision.Logistic > 0.5
	down := decision.LGB < 0.5 && decision.XGB < 0.5 && decision.Logistic < 0.5
	return up || down
}

// Options fixes the statistics of the pre-registered plan
// (docs/plans/20261029-ensemble-member-disagreement.md).
type Options struct {
	PrimarySeed  int
	Seeds        []int
	Bootstrap    int
	Placebos     int
	RNGSeed      int64
	MinTrades    int
	MinDates     int
	MinSegment   int
	WindowOrder  []string
	SeedAgreeMin int
	TercileAgree int
}

func DefaultOptions() Options {
	return Options{
		PrimarySeed:  42,
		Seeds:        []int{0, 1, 2, 3, 4},
		Bootstrap:    5000,
		Placebos:     2000,
		RNGSeed:      42,
		MinTrades:    200,
		MinDates:     40,
		MinSegment:   100,
		WindowOrder:  []string{"q1", "q2", "q3", "q4", "w2", "w1"},
		SeedAgreeMin: 4,
		TercileAgree: 2,
	}
}

type Segment struct {
	Trades   int
	Dates    int
	MeanNet  float64
	TotalNet float64
	HitRate  float64
}

type Contrast struct {
	Diff  float64
	Low   float64
	High  float64
	Draws int
	Skips int
}

type Tercile struct {
	Label       string
	Trades      int
	AgreeTrades int
	SplitTrades int
	Diff        float64
	Low, High   float64
}

type SeedRow struct {
	Seed   int
	Report *Verdict
}

type WindowRow struct {
	Window string
	Agree  Segment
	Split  Segment
	Diff   float64
}

type Verdict struct {
	Rows      int
	Unmatched int
	All       Segment
	Agree     Segment
	Split     Segment
	Primary   Contrast
	Placebo   float64
	Terciles  []Tercile
	PassSize  bool
	PassCI    bool
	PassConv  bool
}

func (v Verdict) Pass() bool { return v.PassSize && v.PassCI }

type Report struct {
	Options  Options
	Overall  Verdict
	Primary  Verdict
	BySeed   []SeedRow
	ByWindow []WindowRow
	Sign     Verdict
	PassSeed bool
	Verdict  string
}

func Analyze(rows []Row, unmatched int, opts Options) Report {
	report := Report{Options: opts}
	report.Overall = analyze(rows, unmatched, opts)

	primary := make([]Row, 0, len(rows))
	bySeed := make(map[int][]Row)
	for _, row := range rows {
		if row.Seed == opts.PrimarySeed {
			primary = append(primary, row)
		}
		bySeed[row.Seed] = append(bySeed[row.Seed], row)
	}
	report.Primary = analyze(primary, unmatched, opts)

	signRows := make([]Row, 0, len(primary))
	for _, row := range primary {
		copied := row
		copied.Unanimous = row.SignAgreement
		signRows = append(signRows, copied)
	}
	report.Sign = analyze(signRows, unmatched, opts)

	seeds := append([]int(nil), opts.Seeds...)
	if len(seeds) == 0 {
		for seed := range bySeed {
			seeds = append(seeds, seed)
		}
		sort.Ints(seeds)
	}
	positive := 0
	for _, seed := range seeds {
		if len(bySeed[seed]) == 0 {
			continue
		}
		verdict := analyze(bySeed[seed], 0, opts)
		report.BySeed = append(report.BySeed, SeedRow{Seed: seed, Report: &verdict})
		if verdict.Primary.Diff > 0 {
			positive++
		}
	}
	report.PassSeed = positive >= opts.SeedAgreeMin

	windows := append([]string(nil), opts.WindowOrder...)
	if len(windows) == 0 {
		seen := map[string]bool{}
		for _, row := range primary {
			if !seen[row.Window] {
				seen[row.Window] = true
				windows = append(windows, row.Window)
			}
		}
		sort.Strings(windows)
	}
	for _, window := range windows {
		subset := make([]Row, 0, 64)
		for _, row := range primary {
			if row.Window == window {
				subset = append(subset, row)
			}
		}
		if len(subset) == 0 {
			continue
		}
		agree, split := segments(subset)
		report.ByWindow = append(report.ByWindow, WindowRow{Window: window, Agree: agree, Split: split, Diff: agree.MeanNet - split.MeanNet})
	}

	switch {
	case report.Primary.Pass() && report.PassSeed && report.Primary.PassConv:
		report.Verdict = "confirmed: split trades are worse per trade; register the abstain filter as a book-level test"
	case report.Primary.Pass() && report.PassSeed:
		report.Verdict = "pass, explained by conviction: the difference does not survive the |p-0.5| terciles, so use conviction instead of an abstain rule and do not wire the filter"
	default:
		report.Verdict = "null: H15 closes, no abstain rule and no sizing change"
	}
	return report
}

func analyze(rows []Row, unmatched int, opts Options) Verdict {
	agree, split := segments(rows)
	verdict := Verdict{
		Rows:      len(rows),
		Unmatched: unmatched,
		All:       segment(rows),
		Agree:     agree,
		Split:     split,
	}
	verdict.Primary = contrast(rows, opts)
	verdict.Placebo = placebo(rows, opts)
	verdict.Terciles = terciles(rows, opts)
	verdict.PassSize = len(rows) >= opts.MinTrades && verdict.All.Dates >= opts.MinDates && agree.Trades >= opts.MinSegment && split.Trades >= opts.MinSegment
	verdict.PassCI = verdict.Primary.Diff > 0 && verdict.Primary.Low > 0
	verdict.PassConv = tercileAgreement(verdict.Terciles, opts.TercileAgree)
	return verdict
}

func tercileAgreement(rows []Tercile, minAgree int) bool {
	agree := 0
	tested := 0
	for _, row := range rows {
		if row.AgreeTrades == 0 || row.SplitTrades == 0 {
			continue
		}
		tested++
		if row.Diff > 0 {
			agree++
		}
	}
	if tested == 0 {
		return false
	}
	if minAgree > tested {
		minAgree = tested
	}
	return agree >= minAgree
}

func segment(rows []Row) Segment {
	out := Segment{Trades: len(rows)}
	if len(rows) == 0 {
		return out
	}
	wins := 0
	dates := map[string]struct{}{}
	var total float64
	for _, row := range rows {
		value := row.NetPnl.InexactFloat64()
		total += value
		if value > 0 {
			wins++
		}
		dates[row.OpenedAt.Format("2006-01-02")] = struct{}{}
	}
	out.TotalNet = total
	out.MeanNet = total / float64(len(rows))
	out.HitRate = float64(wins) / float64(len(rows))
	out.Dates = len(dates)
	return out
}

func segments(rows []Row) (agree, split Segment) {
	a := make([]Row, 0, len(rows))
	s := make([]Row, 0, len(rows))
	for _, row := range rows {
		if row.Unanimous {
			a = append(a, row)
		} else {
			s = append(s, row)
		}
	}
	return segment(a), segment(s)
}

func contrast(rows []Row, opts Options) Contrast {
	out := Contrast{Diff: segmentsDiff(rows)}
	if out.Diff == 0 && len(rows) == 0 {
		return out
	}
	weeks, totals := blocks(rows)
	if len(weeks) < 3 {
		return out
	}
	rng := rand.New(rand.NewSource(opts.RNGSeed))
	diffs := make([]float64, 0, opts.Bootstrap)
	for i := 0; i < opts.Bootstrap; i++ {
		var (
			agreeN, splitN     int
			agreeSum, splitSum float64
		)
		for j := 0; j < len(weeks); j++ {
			pick := rng.Intn(len(weeks))
			t := totals[pick]
			agreeN += t.agreeN
			agreeSum += t.agreeSum
			splitN += t.splitN
			splitSum += t.splitSum
		}
		if agreeN == 0 || splitN == 0 {
			out.Skips++
			continue
		}
		diffs = append(diffs, agreeSum/float64(agreeN)-splitSum/float64(splitN))
	}
	out.Draws = len(diffs)
	if len(diffs) == 0 {
		return out
	}
	sort.Float64s(diffs)
	out.Low = quantile(diffs, 0.025)
	out.High = quantile(diffs, 0.975)
	return out
}

type blockTotals struct {
	agreeN, splitN     int
	agreeSum, splitSum float64
}

func blocks(rows []Row) ([]string, []blockTotals) {
	index := map[string]int{}
	keys := make([]string, 0, 64)
	totals := make([]blockTotals, 0, 64)
	for _, row := range rows {
		year, week := blockKey(row.OpenedAt)
		key := fmt.Sprintf("%04d-%02d", year, week)
		position, ok := index[key]
		if !ok {
			position = len(keys)
			index[key] = position
			keys = append(keys, key)
			totals = append(totals, blockTotals{})
		}
		value := row.NetPnl.InexactFloat64()
		if row.Unanimous {
			totals[position].agreeN++
			totals[position].agreeSum += value
		} else {
			totals[position].splitN++
			totals[position].splitSum += value
		}
	}
	return keys, totals
}

func blockKey(day time.Time) (int, int) {
	return day.ISOWeek()
}

func segmentsDiff(rows []Row) float64 {
	agree, split := segments(rows)
	if agree.Trades == 0 || split.Trades == 0 {
		return 0
	}
	return agree.MeanNet - split.MeanNet
}

func placebo(rows []Row, opts Options) float64 {
	observed := segmentsDiff(rows)
	if observed == 0 || len(rows) == 0 {
		return math.NaN()
	}
	agreeCount := 0
	for _, row := range rows {
		if row.Unanimous {
			agreeCount++
		}
	}
	if agreeCount == 0 || agreeCount == len(rows) {
		return math.NaN()
	}
	rng := rand.New(rand.NewSource(opts.RNGSeed + 1))
	values := make([]float64, len(rows))
	for i, row := range rows {
		values[i] = row.NetPnl.InexactFloat64()
	}
	below := 0
	for i := 0; i < opts.Placebos; i++ {
		order := rng.Perm(len(rows))
		var agreeSum, splitSum float64
		for j := 0; j < agreeCount; j++ {
			agreeSum += values[order[j]]
		}
		for j := agreeCount; j < len(rows); j++ {
			splitSum += values[order[j]]
		}
		diff := agreeSum/float64(agreeCount) - splitSum/float64(len(rows)-agreeCount)
		if diff < observed {
			below++
		}
	}
	return float64(below) / float64(opts.Placebos)
}

func terciles(rows []Row, opts Options) []Tercile {
	if len(rows) == 0 {
		return nil
	}
	sorted := append([]Row(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Conviction() < sorted[j].Conviction() })
	labels := []string{"low conviction", "mid conviction", "high conviction"}
	out := make([]Tercile, 0, 3)
	for i := 0; i < 3; i++ {
		start := i * len(sorted) / 3
		stop := (i + 1) * len(sorted) / 3
		part := sorted[start:stop]
		agree, split := segments(part)
		item := Tercile{Label: labels[i], Trades: len(part), AgreeTrades: agree.Trades, SplitTrades: split.Trades}
		if agree.Trades > 0 && split.Trades > 0 {
			item.Diff = agree.MeanNet - split.MeanNet
			ci := contrast(part, Options{Bootstrap: opts.Bootstrap, RNGSeed: opts.RNGSeed})
			item.Low, item.High = ci.Low, ci.High
		}
		out = append(out, item)
	}
	return out
}

func quantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	position := p * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// Meta records the trades that could not be joined to a decision, so the
// analysis knows how many closed trades a run dropped instead of silently
// treating the join as complete.
type Meta struct {
	Trades    int `json:"trades"`
	Unmatched int `json:"unmatched"`
}

func metaPath(csvPath string) string {
	return csvPath + ".meta.json"
}

func WriteMeta(csvPath string, meta Meta) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("memberagree: marshal meta: %w", err)
	}
	if err := os.WriteFile(metaPath(csvPath), raw, 0o644); err != nil {
		return fmt.Errorf("memberagree: write meta %s: %w", metaPath(csvPath), err)
	}
	return nil
}

func readMeta(csvPath string) Meta {
	var meta Meta
	raw, err := os.ReadFile(metaPath(csvPath))
	if err != nil {
		return meta
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return Meta{}
	}
	return meta
}

func WriteCSV(path string, rows []Row) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("memberagree: create directory: %w", err)
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("memberagree: create %s: %w", path, err)
	}
	defer file.Close()
	writer := csv.NewWriter(file)
	header := []string{"window", "seed", "ticker", "action", "opened_at", "closed_at", "lots", "net_pnl", "ensemble", "lgb", "xgb", "logistic", "unanimous", "sign_agreement"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("memberagree: write header: %w", err)
	}
	for _, row := range rows {
		record := []string{
			row.Window,
			strconv.Itoa(row.Seed),
			row.Ticker,
			row.Action,
			row.OpenedAt.Format("2006-01-02"),
			row.ClosedAt.Format("2006-01-02"),
			strconv.Itoa(row.Lots),
			row.NetPnl.String(),
			strconv.FormatFloat(row.Ensemble, 'f', 6, 64),
			strconv.FormatFloat(row.LGB, 'f', 6, 64),
			strconv.FormatFloat(row.XGB, 'f', 6, 64),
			strconv.FormatFloat(row.Logistic, 'f', 6, 64),
			strconv.FormatBool(row.Unanimous),
			strconv.FormatBool(row.SignAgreement),
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("memberagree: write row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("memberagree: flush %s: %w", path, err)
	}
	return nil
}

func ReadCSV(path string) ([]Row, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("memberagree: open %s: %w", path, err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("memberagree: read %s: %w", path, err)
	}
	if len(records) < 2 {
		return nil, nil
	}
	rows := make([]Row, 0, len(records)-1)
	for line, record := range records[1:] {
		if len(record) != 14 {
			return nil, fmt.Errorf("memberagree: %s line %d: expected 14 fields, got %d", path, line+2, len(record))
		}
		seed, err := strconv.Atoi(record[1])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: seed: %w", path, line+2, err)
		}
		lots, err := strconv.Atoi(record[6])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: lots: %w", path, line+2, err)
		}
		net, err := decimal.NewFromString(record[7])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: net_pnl: %w", path, line+2, err)
		}
		opened, err := time.Parse("2006-01-02", record[4])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: opened_at: %w", path, line+2, err)
		}
		closed, err := time.Parse("2006-01-02", record[5])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: closed_at: %w", path, line+2, err)
		}
		values, err := parseFloats(record[8:12])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: %w", path, line+2, err)
		}
		unanimous, err := strconv.ParseBool(record[12])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: unanimous: %w", path, line+2, err)
		}
		sign, err := strconv.ParseBool(record[13])
		if err != nil {
			return nil, fmt.Errorf("memberagree: %s line %d: sign_agreement: %w", path, line+2, err)
		}
		rows = append(rows, Row{
			Window:        record[0],
			Seed:          seed,
			Ticker:        record[2],
			Action:        record[3],
			OpenedAt:      opened,
			ClosedAt:      closed,
			Lots:          lots,
			NetPnl:        net,
			Ensemble:      values[0],
			LGB:           values[1],
			XGB:           values[2],
			Logistic:      values[3],
			Unanimous:     unanimous,
			SignAgreement: sign,
		})
	}
	return rows, nil
}

func parseFloats(fields []string) ([]float64, error) {
	out := make([]float64, len(fields))
	for i, field := range fields {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return nil, fmt.Errorf("column %d: %w", i, err)
		}
		out[i] = value
	}
	return out, nil
}

// LoadDir reads every *.csv file of a directory in name order and sums the
// unmatched counts of their sidecar metadata files.
func LoadDir(dir string) ([]Row, int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, fmt.Errorf("memberagree: read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".csv") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	var rows []Row
	unmatched := 0
	for _, name := range names {
		path := filepath.Join(dir, name)
		part, err := ReadCSV(path)
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, part...)
		unmatched += readMeta(path).Unmatched
	}
	return rows, unmatched, nil
}

// LoadPath reads a single CSV file or every CSV of a directory.
func LoadPath(path string) ([]Row, int, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("memberagree: stat %s: %w", path, err)
	}
	if info.IsDir() {
		return LoadDir(path)
	}
	rows, err := ReadCSV(path)
	if err != nil {
		return nil, 0, err
	}
	return rows, readMeta(path).Unmatched, nil
}

func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Member agreement of the ensemble (H15)\n\n")
	fmt.Fprintf(&b, "Rows: %d closed trades, %d unmatched (no decision).\n\n", r.Overall.Rows, r.Overall.Unmatched)
	fmt.Fprintf(&b, "### Primary arm, seed %d\n\n", r.Options.PrimarySeed)
	writeSegmentTable(&b, r.Primary)
	fmt.Fprintf(&b, "\nDifference unanimous - split: %+.2f RUB per trade, 95%% cluster bootstrap [%+.2f, %+.2f], %d draws, %d skipped.\n",
		r.Primary.Primary.Diff, r.Primary.Primary.Low, r.Primary.Primary.High, r.Primary.Primary.Draws, r.Primary.Primary.Skips)
	fmt.Fprintf(&b, "Placebo percentile of the observed difference: %.3f (%d permutations).\n", r.Primary.Placebo, r.Options.Placebos)
	fmt.Fprintf(&b, "Size gate: %d trades (min %d), %d dates (min %d), segments %d/%d (min %d): %s.\n",
		r.Primary.Rows, r.Options.MinTrades, r.Primary.All.Dates, r.Options.MinDates,
		r.Primary.Agree.Trades, r.Primary.Split.Trades, r.Options.MinSegment, passWord(r.Primary.PassSize))
	fmt.Fprintf(&b, "Interval gate: %s. Conviction gate: %s.\n\n", passWord(r.Primary.PassCI), passWord(r.Primary.PassConv))
	fmt.Fprintf(&b, "| conviction tercile | trades | unanimous | split | difference, RUB per trade | 95%% CI |\n|---|---|---|---|---|---|\n")
	for _, tercile := range r.Primary.Terciles {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %+.2f | [%+.2f, %+.2f] |\n",
			tercile.Label, tercile.Trades, tercile.AgreeTrades, tercile.SplitTrades, tercile.Diff, tercile.Low, tercile.High)
	}
	fmt.Fprintf(&b, "\n### Seeds\n\n| seed | trades | unanimous | split | difference | 95%% CI |\n|---|---|---|---|---|---|\n")
	for _, row := range r.BySeed {
		verdict := row.Report
		fmt.Fprintf(&b, "| %d | %d | %d | %d | %+.2f | [%+.2f, %+.2f] |\n",
			row.Seed, verdict.Rows, verdict.Agree.Trades, verdict.Split.Trades, verdict.Primary.Diff, verdict.Primary.Low, verdict.Primary.High)
	}
	fmt.Fprintf(&b, "\nSeed gate (sign in at least %d of %d): %s.\n\n", r.Options.SeedAgreeMin, len(r.BySeed), passWord(r.PassSeed))
	fmt.Fprintf(&b, "### Windows, seed %d\n\n| window | unanimous trades | split trades | difference | unanimous mean | split mean |\n|---|---|---|---|---|---|\n", r.Options.PrimarySeed)
	for _, row := range r.ByWindow {
		fmt.Fprintf(&b, "| %s | %d | %d | %+.2f | %+.2f | %+.2f |\n",
			row.Window, row.Agree.Trades, row.Split.Trades, row.Diff, row.Agree.MeanNet, row.Split.MeanNet)
	}
	fmt.Fprintf(&b, "\n### Sign agreement (descriptive)\n\n")
	writeSegmentTable(&b, r.Sign)
	fmt.Fprintf(&b, "\nDifference (sign agreement - not): %+.2f RUB per trade, 95%% CI [%+.2f, %+.2f].\n", r.Sign.Primary.Diff, r.Sign.Primary.Low, r.Sign.Primary.High)
	fmt.Fprintf(&b, "\n**Verdict: %s.**\n", r.Verdict)
	return b.String()
}

func writeSegmentTable(b *strings.Builder, verdict Verdict) {
	fmt.Fprintf(b, "| segment | trades | open dates | mean net RUB | total net RUB | hit rate |\n|---|---|---|---|---|---|\n")
	for _, item := range []struct {
		name    string
		segment Segment
	}{{"unanimous", verdict.Agree}, {"split", verdict.Split}, {"all", verdict.All}} {
		fmt.Fprintf(b, "| %s | %d | %d | %+.2f | %+.2f | %.3f |\n",
			item.name, item.segment.Trades, item.segment.Dates, item.segment.MeanNet, item.segment.TotalNet, item.segment.HitRate)
	}
}

func passWord(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}
